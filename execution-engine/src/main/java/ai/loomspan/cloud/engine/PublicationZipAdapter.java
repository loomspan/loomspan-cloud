package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillDocument;
import jakarta.servlet.http.Part;
import org.apache.commons.compress.archivers.zip.ZipArchiveEntry;
import org.apache.commons.compress.archivers.zip.ZipFile;
import org.apache.commons.compress.utils.SeekableInMemoryByteChannel;
import org.snakeyaml.engine.v2.api.LoadSettings;
import org.snakeyaml.engine.v2.api.lowlevel.Compose;
import org.snakeyaml.engine.v2.exceptions.YamlEngineException;
import org.snakeyaml.engine.v2.nodes.MappingNode;
import org.snakeyaml.engine.v2.nodes.Node;
import org.snakeyaml.engine.v2.nodes.ScalarNode;
import org.snakeyaml.engine.v2.nodes.Tag;
import org.snakeyaml.engine.v2.schema.CoreSchema;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.zip.CRC32;

@Component
final class PublicationZipAdapter {
    static final int MAX_ZIP_BYTES = 1_000_000;
    static final int MAX_EXPANDED_BYTES = 10_000_000;
    static final int MAX_ENTRIES = 1_000;
    private static final int CHUNK_SIZE = 8_192;
    private static final int LOCAL_HEADER_SIZE = 30;
    private static final int LOCAL_SIGNATURE = 0x04034b50;
    private static final int DOS_DIRECTORY = 0x10;
    private static final int DOS_VOLUME_LABEL = 0x08;
    private static final int UNIX_TYPE_MASK = 0170000;
    private static final int UNIX_REGULAR = 0100000;
    private static final int UNIX_DIRECTORY = 0040000;
    private static final String EXECUTION_PATH = "loomspan/execution.yaml";
    private static final Charset LEGACY_ZIP_CHARSET = Charset.forName("IBM437");
    private static final MediaType ZIP_MEDIA_TYPE = MediaType.valueOf("application/zip");
    private static final LoadSettings YAML_SETTINGS = LoadSettings.builder()
            .setCodePointLimit(MAX_EXPANDED_BYTES)
            .setSchema(new CoreSchema())
            .build();

    AdaptedPublication adapt(String namespace, Part pack) {
        if (pack == null || !"pack".equals(pack.getName()) || pack.getContentType() == null
                || !ZIP_MEDIA_TYPE.equalsTypeAndSubtype(MediaType.parseMediaType(pack.getContentType()))) {
            throw new IllegalArgumentException("Expected an application/zip multipart part named pack");
        }
        if (namespace == null || namespace.isBlank()) {
            throw new IllegalArgumentException("Expected a publication namespace");
        }
        try {
            byte[] bytes = readZip(pack);
            var directory = ZipDirectoryPreflight.inspect(bytes, MAX_ENTRIES);
            try (var channel = new SeekableInMemoryByteChannel(directory.readerArchive(bytes))) {
                try (ZipFile zip = ZipFile.builder()
                        .setSeekableByteChannel(channel)
                        .setCharset(LEGACY_ZIP_CHARSET)
                        .setUseUnicodeExtraFields(false)
                        .setIgnoreLocalFileHeader(true)
                        .get()) {
                    return extract(namespace, bytes, directory, zip);
                }
            }
        } catch (IOException exception) {
            throw new IllegalArgumentException("Unable to read publication ZIP", exception);
        }
    }

    private static byte[] readZip(Part pack) throws IOException {
        if (pack.getSize() > MAX_ZIP_BYTES) {
            throw zipLimit();
        }
        var bytes = new ByteArrayOutputStream();
        byte[] chunk = new byte[CHUNK_SIZE];
        try (var input = pack.getInputStream()) {
            int read;
            while ((read = input.read(chunk, 0, Math.min(chunk.length, MAX_ZIP_BYTES - bytes.size() + 1))) != -1) {
                if (read > MAX_ZIP_BYTES - bytes.size()) {
                    throw zipLimit();
                }
                bytes.write(chunk, 0, read);
            }
        }
        return bytes.toByteArray();
    }

    private static IllegalArgumentException zipLimit() {
        return new IllegalArgumentException("Publication ZIP exceeds " + MAX_ZIP_BYTES + " bytes");
    }

    private static AdaptedPublication extract(String namespace, byte[] bytes, ZipDirectoryPreflight.Directory directory,
                                              ZipFile zip) {
        var archive = ByteBuffer.wrap(bytes).order(ByteOrder.LITTLE_ENDIAN);
        var executionEntries = new ArrayList<ExecutionEntry>();
        int companions = 0;
        int declaredTotal = 0;
        int count = 0;
        var entries = zip.getEntries();
        while (entries.hasMoreElements()) {
            var entry = entries.nextElement();
            count++;
            String path = authoredPath(entry);
            checkLocalHeader(archive, directory.startOffset(), entry, path);
            String relative = relativePath(namespace, path);
            long declared = entry.getSize();
            if (declared < 0 || declared > MAX_EXPANDED_BYTES - declaredTotal) {
                throw expandedLimit(path);
            }
            declaredTotal += (int) declared;
            if (relative.isEmpty()) {
                checkType(entry, path, true);
            }
            if (isExecutionPath(relative)) {
                executionEntries.add(new ExecutionEntry(relative, entry));
            } else {
                // A seekable reader leaves companion payloads compressed.
                // Staging verifies their actual sizes before submitting a pack.
                companions += (int) declared;
            }
        }
        if (count != directory.entries()) {
            throw new IllegalArgumentException("Inconsistent publication ZIP entry count");
        }
        checkExecutionCollisions(executionEntries);
        for (var entry : executionEntries) {
            checkType(entry.zipEntry(), entry.path(), entry.directory());
            checkExecutionLayout(entry);
        }
        return readExecution(zip, executionEntries, companions);
    }

    private static String authoredPath(ZipArchiveEntry entry) {
        Charset charset = entry.getGeneralPurposeBit().usesUTF8ForNames()
                ? StandardCharsets.UTF_8 : LEGACY_ZIP_CHARSET;
        try {
            String name = charset.newDecoder().decode(ByteBuffer.wrap(entry.getRawName())).toString();
            if (!name.equals(entry.getName())) {
                throw invalidPath(name, "ZIP entry name was normalized");
            }
            return name;
        } catch (CharacterCodingException exception) {
            throw invalidPath(entry.getName(), "invalid ZIP entry name encoding");
        }
    }

    private static void checkLocalHeader(ByteBuffer archive, int centralStart, ZipArchiveEntry entry, String path) {
        long offset = entry.getLocalHeaderOffset();
        if (entry.getDiskNumberStart() != 0 || offset < 0 || offset > centralStart - LOCAL_HEADER_SIZE) {
            throw invalidPath(path, "invalid ZIP local header offset");
        }
        int local = (int) offset;
        int nameLength = Short.toUnsignedInt(archive.getShort(local + 26));
        int extraLength = Short.toUnsignedInt(archive.getShort(local + 28));
        int nameStart = local + LOCAL_HEADER_SIZE;
        int nameEnd = nameStart + nameLength;
        int dataStart = nameEnd + extraLength;
        long compressed = entry.getCompressedSize();
        if (archive.getInt(local) != LOCAL_SIGNATURE || dataStart > centralStart
                || compressed < 0 || compressed > centralStart - dataStart) {
            throw invalidPath(path, "invalid ZIP local entry bounds");
        }
        byte[] centralName = entry.getRawName();
        if (!Arrays.equals(archive.array(), nameStart, nameEnd, centralName, 0, centralName.length)) {
            throw invalidPath(path, "ZIP local name differs from the central name");
        }
        int localFlags = Short.toUnsignedInt(archive.getShort(local + 6));
        int localMethod = Short.toUnsignedInt(archive.getShort(local + 8));
        if (localFlags != entry.getRawFlag() || localMethod != entry.getMethod()) {
            throw invalidPath(path, "ZIP local flags or method differ from the central entry");
        }
    }

    private static String relativePath(String namespace, String path) {
        if (path.indexOf('\\') >= 0 || path.codePoints().anyMatch(Character::isISOControl)) {
            throw invalidPath(path, "ambiguous separator or control character");
        }
        String withoutTrailingSlash = path.endsWith("/") ? path.substring(0, path.length() - 1) : path;
        for (String component : withoutTrailingSlash.split("/", -1)) {
            if (component.isEmpty() || component.equals(".") || component.equals("..")) {
                throw invalidPath(path, "ambiguous path component '" + component + "'");
            }
        }
        int firstSeparator = path.indexOf('/');
        if (firstSeparator < 0 || !namespace.equals(path.substring(0, firstSeparator))) {
            throw invalidPath(path, "expected a single top-level directory '" + namespace + "'");
        }
        return path.substring(firstSeparator + 1);
    }

    private static boolean isExecutionPath(String relative) {
        int separator = relative.indexOf('/');
        return "loomspan".equalsIgnoreCase(separator < 0 ? relative : relative.substring(0, separator));
    }

    private static void checkType(ZipArchiveEntry entry, String path, boolean directory) {
        long attributes = entry.getExternalAttributes();
        int unixType = entry.getUnixMode() & UNIX_TYPE_MASK;
        if ((attributes & DOS_VOLUME_LABEL) != 0
                || (unixType != 0 && unixType != (directory ? UNIX_DIRECTORY : UNIX_REGULAR))
                || (!directory && (attributes & DOS_DIRECTORY) != 0)
                || (directory && entry.getSize() != 0)) {
            throw invalidPath(path, directory ? "expected a directory" : "expected a regular file");
        }
    }

    private static void checkExecutionCollisions(List<ExecutionEntry> entries) {
        // Sort by folded path components so every shared ancestor is a
        // contiguous group, without materializing its implied directories.
        var sorted = new ArrayList<>(entries);
        sorted.sort((left, right) -> Arrays.compare(left.components(), right.components(), String.CASE_INSENSITIVE_ORDER));
        for (int index = 1; index < sorted.size(); index++) {
            var first = sorted.get(index - 1);
            var second = sorted.get(index);
            int shared = 0;
            while (shared < first.components().length && shared < second.components().length
                    && first.components()[shared].equalsIgnoreCase(second.components()[shared])) {
                if (!first.components()[shared].equals(second.components()[shared])) {
                    throw collision(first, second);
                }
                shared++;
            }
            if (shared == first.components().length
                    && (shared == second.components().length || !first.directory())) {
                throw collision(first, second);
            }
        }
    }

    private static IllegalArgumentException collision(ExecutionEntry first, ExecutionEntry second) {
        return invalidPath(second.path(), "case-insensitive execution path collision with '" + first.path() + "'");
    }

    private static void checkExecutionLayout(ExecutionEntry entry) {
        String path = entry.path();
        String[] components = entry.components();
        boolean skillTree = components.length >= 2 && components[1].equalsIgnoreCase("skills");
        if (skillTree) {
            for (String component : components) {
                if (!component.equals(component.toLowerCase(Locale.ROOT))) {
                    throw invalidPath(path, "expected lowercase skill path component '" + component + "'");
                }
            }
            if (entry.directory() || (components.length >= 3 && path.endsWith(".yaml"))) {
                return;
            }
        }
        if ((!entry.directory() && path.equals(EXECUTION_PATH))
                || (entry.directory() && path.equals("loomspan/"))) {
            return;
        }
        throw invalidPath(path, "invalid execution layout");
    }

    private static AdaptedPublication readExecution(ZipFile zip, List<ExecutionEntry> entries, int companions) {
        byte[] chunk = new byte[CHUNK_SIZE];
        String executionYaml = null;
        var skills = new ArrayList<SkillDocument>();
        int remaining = MAX_EXPANDED_BYTES - companions;
        for (var entry : entries) {
            if (entry.directory()) {
                checkEmptyDirectory(zip, entry);
                continue;
            }
            var bytes = readEntry(zip, entry, remaining, chunk);
            remaining -= bytes.length;
            String yaml;
            try {
                yaml = StandardCharsets.UTF_8.newDecoder().decode(ByteBuffer.wrap(bytes)).toString();
            } catch (CharacterCodingException exception) {
                throw invalidPath(entry.path(), "invalid UTF-8 YAML");
            }
            if (entry.path().equals(EXECUTION_PATH)) {
                checkReservedFields(yaml);
                executionYaml = yaml;
            } else {
                skills.add(new SkillDocument(entry.path(), yaml));
            }
        }
        if (executionYaml == null) {
            throw invalidPath(EXECUTION_PATH, "required execution configuration is missing");
        }
        return new AdaptedPublication(executionYaml, skills);
    }

    private static void checkEmptyDirectory(ZipFile zip, ExecutionEntry entry) {
        try (var input = zip.getInputStream(entry.zipEntry())) {
            if (input.read() != -1) {
                throw invalidPath(entry.path(), "directory contains file data");
            }
        } catch (IOException exception) {
            throw new IllegalArgumentException("Unable to read ZIP entry '" + entry.path() + "'", exception);
        }
    }

    private static byte[] readEntry(ZipFile zip, ExecutionEntry entry, int allowance, byte[] chunk) {
        var bytes = new ByteArrayOutputStream();
        var crc = new CRC32();
        try (var input = zip.getInputStream(entry.zipEntry())) {
            int read;
            while ((read = input.read(chunk, 0, Math.min(chunk.length, allowance - bytes.size() + 1))) != -1) {
                if (read > allowance - bytes.size()) {
                    throw expandedLimit(entry.path());
                }
                bytes.write(chunk, 0, read);
                crc.update(chunk, 0, read);
            }
        } catch (IOException exception) {
            throw new IllegalArgumentException("Unable to read ZIP entry '" + entry.path() + "'", exception);
        }
        if (bytes.size() != entry.zipEntry().getSize() || crc.getValue() != entry.zipEntry().getCrc()) {
            throw invalidPath(entry.path(), "expanded size or CRC does not match the ZIP directory");
        }
        return bytes.toByteArray();
    }

    private static void checkReservedFields(String yaml) {
        Node root;
        try {
            root = new Compose(YAML_SETTINGS).composeString(yaml).orElse(null);
        } catch (YamlEngineException exception) {
            throw new IllegalArgumentException("Invalid YAML in '" + EXECUTION_PATH + "'", exception);
        } catch (StackOverflowError exception) {
            throw new IllegalArgumentException("Excessive YAML nesting in '" + EXECUTION_PATH + "'", exception);
        }
        var reserved = new LinkedHashSet<String>();
        if (root instanceof MappingNode mapping) {
            for (var entry : mapping.getValue()) {
                if ("loomspan".equals(stringKey(entry.getKeyNode())) && entry.getValueNode() instanceof MappingNode config) {
                    for (var field : config.getValue()) {
                        String key = stringKey(field.getKeyNode());
                        if ("session".equals(key) || "execution-trace".equals(key)) {
                            reserved.add("loomspan." + key);
                        }
                    }
                }
            }
        }
        if (!reserved.isEmpty()) {
            throw invalidPath(EXECUTION_PATH, "reserved configuration fields " + String.join(", ", reserved));
        }
    }

    private static String stringKey(Node node) {
        return node instanceof ScalarNode scalar && Tag.STR.equals(scalar.getTag()) ? scalar.getValue() : null;
    }

    private static IllegalArgumentException expandedLimit(String path) {
        return invalidPath(path, "publication ZIP exceeds " + MAX_EXPANDED_BYTES + " expanded bytes");
    }

    private static IllegalArgumentException invalidPath(String path, String reason) {
        return new IllegalArgumentException("ZIP entry '" + path + "': " + reason);
    }

    private record ExecutionEntry(String path, ZipArchiveEntry zipEntry, String[] components) {
        ExecutionEntry(String path, ZipArchiveEntry zipEntry) {
            this(path, zipEntry, path.split("/"));
        }

        boolean directory() {
            return zipEntry.isDirectory();
        }
    }
}
