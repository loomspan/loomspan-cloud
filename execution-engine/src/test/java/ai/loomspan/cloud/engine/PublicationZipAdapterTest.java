package ai.loomspan.cloud.engine;

import ai.loomspan.api.SkillDocument;
import org.apache.commons.compress.archivers.zip.ZipArchiveEntry;
import org.apache.commons.compress.archivers.zip.ZipArchiveOutputStream;
import org.apache.commons.compress.archivers.zip.Zip64Mode;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.junit.jupiter.params.provider.ValueSource;
import org.springframework.mock.web.MockPart;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import java.util.zip.CRC32;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatIllegalArgumentException;

class PublicationZipAdapterTest {
    private static final String NAMESPACE = "milestone";
    private static final String CONFIG_PATH = NAMESPACE + "/loomspan/execution.yaml";
    private static final String CONFIG = "loomspan: {}\n";
    private static final String SKILL_PATH = NAMESPACE + "/loomspan/skills/echo-release.yaml";
    private static final String COMPANION_PATH = NAMESPACE + "/references/readme.md";
    private final PublicationZipAdapter adapter = new PublicationZipAdapter();

    @Test
    void extractsOnlyExecutionInputsWithOriginalSourceNames() throws IOException {
        String execution = """
                loomspan:
                  executor-config:
                    unrelated-owner:
                      service: &endpoint {$var: app.endpoint}
                      other: *endpoint
                """;
        String authoredSkill = "name: find-order\nexecutor: unknown:executor\n";
        var result = adapter.adapt(NAMESPACE, part(zip(Zip64Mode.AsNeeded,
                entry(NAMESPACE + "/", ""), entry(CONFIG_PATH, execution),
                entry(NAMESPACE + "/References/Readme.md", "not YAML ["),
                entry(NAMESPACE + "/loomspan/skills/orders/find-order.yaml", authoredSkill),
                entry(NAMESPACE + "/loomspan/skills/orders/", ""),
                entry(NAMESPACE + "/loomspan/skills/echo-release.yaml", "name: echo-release"))));

        assertThat(result.executionYaml()).isEqualTo(execution);
        assertThat(result.skills()).containsExactly(
                new SkillDocument("loomspan/skills/orders/find-order.yaml", authoredSkill),
                new SkillDocument("loomspan/skills/echo-release.yaml", "name: echo-release"));
    }

    @Test
    void acceptsPortablePackWithoutSkills() throws IOException {
        byte[] archive = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG));
        assertThat(adapter.adapt(NAMESPACE, part(archive))).isEqualTo(new AdaptedPublication(CONFIG, List.of()));
    }

    @Test
    void enforcesZipBytesWhileReading() throws IOException {
        int max = PublicationZipAdapter.MAX_ZIP_BYTES;
        assertThat(adapter.adapt(NAMESPACE, part(archiveWithSize(max))).executionYaml()).isEqualTo(CONFIG);

        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(archiveWithSize(max + 1))))
                .withMessageContaining("1000000 bytes");

        byte[] longer = archiveWithSize(max + 32_768);
        var counting = new ByteArrayInputStream(longer) {
            int consumed() {
                return pos;
            }
        };
        var underreported = new MockPart("pack", "publication.zip", new byte[0]) {
            @Override
            public InputStream getInputStream() {
                return counting;
            }
        };
        underreported.getHeaders().set("Content-Type", "application/zip");
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, underreported))
                .withMessageContaining("1000000 bytes");
        assertThat(counting.consumed()).isEqualTo(max + 1);
    }

    @Test
    void countsActualCentralEntriesIncludingDirectoriesBeforeIndexing() throws IOException {
        var entries = new Entry[PublicationZipAdapter.MAX_ENTRIES + 1];
        entries[0] = entry(CONFIG_PATH, CONFIG);
        for (int index = 1; index < entries.length; index++) {
            entries[index] = entry(NAMESPACE + "/references/directory-" + index + "/", "");
        }
        byte[] exact = zip(Zip64Mode.AsNeeded, Arrays.copyOf(entries, entries.length - 1));
        assertThat(ZipDirectoryPreflight.inspect(exact, PublicationZipAdapter.MAX_ENTRIES).entries()).isEqualTo(1000);
        assertThat(adapter.adapt(NAMESPACE, part(exact)).executionYaml()).isEqualTo(CONFIG);

        byte[] above = zip(Zip64Mode.AsNeeded, entries);
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(above)))
                .withMessageContaining("1000 entries");
        var directory = littleEndian(above);
        int end = endOffset(above);
        directory.putShort(end + 8, (short) 1).putShort(end + 10, (short) 1);
        assertThatIllegalArgumentException().isThrownBy(() -> ZipDirectoryPreflight.inspect(above, 1_000))
                .withMessageContaining("1000 entries");
    }

    @Test
    void rejectsAnInvalidCentralDirectoryAndAcceptsSingleDiskZip64() throws IOException {
        byte[] corrupt = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG));
        littleEndian(corrupt).putShort(centralOffset(corrupt) + 28, (short) 0xffff);
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(corrupt)))
                .withMessageContaining("central directory");
        assertThat(adapter.adapt(NAMESPACE, part(zip(Zip64Mode.Always, entry(CONFIG_PATH, CONFIG)))).executionYaml())
                .isEqualTo(CONFIG);
    }

    @Test
    void ignoresEndRecordSignaturesInAnArchiveComment() throws IOException {
        byte[] original = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG));
        byte[] withComment = Arrays.copyOf(original, original.length + 60);
        int signatureInComment = original.length + 7;
        littleEndian(withComment).putShort(endOffset(original) + 20, (short) 60)
                .putInt(signatureInComment, 0x06054b50)
                .putShort(signatureInComment + 20, (short) (withComment.length - signatureInComment - 22));
        assertThat(adapter.adapt(NAMESPACE, part(withComment)).executionYaml()).isEqualTo(CONFIG);
    }

    @Test
    void ignoresZip64LocatorSignatureInAnEntryComment() throws IOException {
        byte[] original = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG));
        int oldEnd = endOffset(original);
        int commentBytes = 20;
        byte[] withComment = Arrays.copyOf(original, original.length + commentBytes);
        System.arraycopy(original, oldEnd, withComment, oldEnd + commentBytes, original.length - oldEnd);
        Arrays.fill(withComment, oldEnd, oldEnd + commentBytes, (byte) 0);
        littleEndian(withComment).putShort(centralOffset(original) + 32, (short) commentBytes)
                .putInt(oldEnd, 0x07064b50)
                .putInt(endOffset(withComment) + 12, oldEnd - centralOffset(original) + commentBytes);
        assertThat(adapter.adapt(NAMESPACE, part(withComment)).executionYaml()).isEqualTo(CONFIG);
    }

    @Test
    void enforcesExpandedBytesBeforeRetainingEachChunk() throws IOException {
        int maxSkillBytes = PublicationZipAdapter.MAX_EXPANDED_BYTES - CONFIG.length();
        byte[] exact = zip(Zip64Mode.AsNeeded, entry(SKILL_PATH, "a".repeat(maxSkillBytes)), entry(CONFIG_PATH, CONFIG));
        String skill = adapter.adapt(NAMESPACE, part(exact)).skills().getFirst().yaml();
        assertThat(skill.length()).isEqualTo(maxSkillBytes);

        byte[] above = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), entry(SKILL_PATH, "a".repeat(maxSkillBytes + 1)));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(above)))
                .withMessageContaining(SKILL_PATH).withMessageContaining("10000000 expanded bytes");

        byte[] forged = zip(Zip64Mode.AsNeeded, entry(SKILL_PATH, "a".repeat(PublicationZipAdapter.MAX_EXPANDED_BYTES + 1)),
                entry(CONFIG_PATH, CONFIG));
        littleEndian(forged).putInt(centralOffset(forged) + 24, 1);
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(forged)))
                .withMessageContaining("loomspan/skills/echo-release.yaml").withMessageContaining("10000000 expanded bytes");
    }

    @Test
    void skipsCompanionPayloadsAndAccountsForTheirDeclaredSize() throws IOException {
        byte[] archive = zip(Zip64Mode.AsNeeded, new Entry(COMPANION_PATH, new byte[8], 0, true),
                entry(CONFIG_PATH, CONFIG));
        int firstDataOffset = 30 + COMPANION_PATH.length();
        archive[firstDataOffset] = 42; // The companion's CRC no longer matches its bytes.
        assertThat(adapter.adapt(NAMESPACE, part(archive)).executionYaml()).isEqualTo(CONFIG);
        int central = centralOffset(archive);
        littleEndian(archive).putInt(central + 24, PublicationZipAdapter.MAX_EXPANDED_BYTES);
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(archive)))
                .withMessageContaining("10000000 expanded bytes");
    }

    @Test
    void rejectsDataInAnExecutionDirectoryDeclaredEmpty() throws IOException {
        String directory = NAMESPACE + "/loomspan/skills/";
        byte[] archive = zip(Zip64Mode.AsNeeded, entry(directory, "not empty"), entry(CONFIG_PATH, CONFIG));
        littleEndian(archive).putInt(centralOffset(archive) + 24, 0);
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(archive)))
                .withMessageContaining("loomspan/skills/").withMessageContaining("directory contains file data");
    }

    @ParameterizedTest
    @CsvSource({
            "milestone/references/../loomspan/execution.yaml, ambiguous path component",
            "/milestone/file, ambiguous path component",
            "milestone/references//file, ambiguous path component",
            "milestone/references/./file, ambiguous path component",
            "other/references/file, expected a single top-level directory",
            "milestone, expected a single top-level directory",
            "milestone/loomspan, case-insensitive execution path collision",
            "milestone/loomspan/Execution.yaml, case-insensitive execution path collision",
            "milestone/loomspan/execution.yml, invalid execution layout",
            "milestone/loomspan/random.yaml, invalid execution layout",
            "milestone/loomspan/skills, invalid execution layout",
            "milestone/loomspan/skills/order.yml, invalid execution layout",
            "milestone/loomspan/skills/Orders/find-order.yaml, Orders",
            "milestone/Loomspan/skills/orders/find-order.yaml, Loomspan"
    })
    void rejectsInvalidPathsAndLayout(String path, String reason) throws IOException {
        byte[] archive = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), entry(path, ""));
        String reportedPath = path.toLowerCase(Locale.ROOT).startsWith("milestone/loomspan")
                ? path.substring(NAMESPACE.length() + 1) : path;
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(archive)))
                .withMessageContaining(reportedPath).withMessageContaining(reason);
    }

    @Test
    void rejectsLocalAndCentralNameConflictsAndBackslashes() throws IOException {
        byte[] malformed = zip(Zip64Mode.AsNeeded, entry(COMPANION_PATH, "ignore"), entry(CONFIG_PATH, CONFIG));
        malformed[30 + NAMESPACE.length()] = '\\';
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(malformed)))
                .withMessageContaining(COMPANION_PATH).withMessageContaining("ZIP local name differs");

        malformed[centralOffset(malformed) + 46 + NAMESPACE.length()] = '\\';
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(malformed)))
                .withMessageContaining("milestone\\references/readme.md");
    }

    @Test
    void confinesTheRequestedNamespaceToOneTopLevelComponent() throws IOException {
        byte[] archive = zip(Zip64Mode.AsNeeded, entry("milestone/nested/loomspan/execution.yaml", CONFIG));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt("milestone/nested", part(archive)))
                .withMessageContaining("expected a single top-level directory");
    }

    @Test
    void preservesCaseSensitiveUnicodeSourceNamesAndRejectsCaseFoldedCollisions() throws IOException {
        String skillRoot = NAMESPACE + "/loomspan/skills/";
        byte[] oneSkill = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), entry(skillRoot + "σ.yaml", "skill"));
        assertThat(adapter.adapt(NAMESPACE, part(oneSkill)).skills().getFirst().sourceName())
                .isEqualTo("loomspan/skills/σ.yaml");

        byte[] conflicting = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG),
                entry(skillRoot + "σ.yaml", "skill"), entry(skillRoot + "ς.yaml", "skill"));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(conflicting)))
                .withMessageContaining("loomspan/skills/σ.yaml").withMessageContaining("loomspan/skills/ς.yaml");
    }

    @Test
    void rejectsMissingConfigNonregularExecutionFilesAndPathCollisions() throws IOException {
        byte[] withoutConfig = zip(Zip64Mode.AsNeeded, entry(COMPANION_PATH, ""));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(withoutConfig)))
                .withMessageContaining("loomspan/execution.yaml").withMessageContaining("missing");

        var symlink = new Entry(SKILL_PATH, "elsewhere".getBytes(StandardCharsets.UTF_8), 0120777, false);
        byte[] nonregular = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), symlink);
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(nonregular)))
                .withMessageContaining("loomspan/skills/echo-release.yaml").withMessageContaining("regular file");

        byte[] duplicates = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), entry(CONFIG_PATH, CONFIG));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(duplicates)))
                .withMessageContaining("loomspan/execution.yaml").withMessageContaining("collision");

        byte[] implied = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG),
                entry(NAMESPACE + "/loomspan/skills/orders/first.yaml", ""),
                entry(NAMESPACE + "/loomspan/skills/Orders/second.yaml", ""));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(implied)))
                .withMessageContaining("loomspan/skills/orders/first.yaml")
                .withMessageContaining("loomspan/skills/Orders/second.yaml");

        byte[] fileAsParent = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), entry(SKILL_PATH, ""),
                entry(SKILL_PATH + "/nested.yaml", ""));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(fileAsParent)))
                .withMessageContaining("loomspan/skills/echo-release.yaml/nested.yaml").withMessageContaining("collision");
    }

    @ParameterizedTest
    @ValueSource(strings = {
            "loomspan: {session: {}, execution-trace: null}",
            "template: &template {'session': null, 'execution-trace': {}}\nloomspan: *template"
    })
    void rejectsAuthoredReservedSectionsByFieldPath(String config) throws IOException {
        byte[] archive = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, config));
        assertThatIllegalArgumentException().isThrownBy(() -> adapter.adapt(NAMESPACE, part(archive)))
                .withMessageContaining("loomspan.session").withMessageContaining("loomspan.execution-trace");
    }

    @Test
    void permitsNonreservedNestedFieldsAndPreservesTheirText() throws IOException {
        String config = "loomspan: {executor-config: {milestone: {session: {}}}}\n";
        assertThat(adapter.adapt(NAMESPACE, part(zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, config)))).executionYaml())
                .isEqualTo(config);
    }

    static byte[] archiveWithSize(int size) throws IOException {
        byte[] empty = zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), new Entry(COMPANION_PATH, new byte[0], 0, true));
        return zip(Zip64Mode.AsNeeded, entry(CONFIG_PATH, CONFIG), new Entry(COMPANION_PATH, new byte[size - empty.length], 0, true));
    }

    static MockPart part(byte[] archive) {
        var part = new MockPart("pack", "publication.zip", archive);
        part.getHeaders().set("Content-Type", "application/zip");
        return part;
    }

    private static Entry entry(String path, String data) {
        return new Entry(path, data.getBytes(StandardCharsets.UTF_8), 0, false);
    }

    private static byte[] zip(Zip64Mode mode, Entry... entries) throws IOException {
        var bytes = new ByteArrayOutputStream();
        try (var archive = new ZipArchiveOutputStream(bytes)) {
            archive.setUseZip64(mode);
            for (var entry : entries) {
                var header = new ZipArchiveEntry(entry.path());
                if (entry.unixMode() != 0) {
                    header.setUnixMode(entry.unixMode());
                }
                if (entry.stored()) {
                    var crc = new CRC32();
                    crc.update(entry.data());
                    header.setMethod(ZipArchiveEntry.STORED);
                    header.setSize(entry.data().length);
                    header.setCrc(crc.getValue());
                }
                archive.putArchiveEntry(header);
                archive.write(entry.data());
                archive.closeArchiveEntry();
            }
        }
        return bytes.toByteArray();
    }

    private static ByteBuffer littleEndian(byte[] archive) {
        return ByteBuffer.wrap(archive).order(ByteOrder.LITTLE_ENDIAN);
    }

    private static int centralOffset(byte[] archive) {
        return littleEndian(archive).getInt(endOffset(archive) + 16);
    }

    private static int endOffset(byte[] archive) {
        return archive.length - 22;
    }

    private record Entry(String path, byte[] data, int unixMode, boolean stored) {}
}
