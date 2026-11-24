package ai.loomspan.cloud.engine;

import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.Arrays;

/**
 * Counts actual central-directory records before opening a ZIP reader that
 * indexes every entry. All offsets are bounded by the received archive.
 */
final class ZipDirectoryPreflight {
    private static final int END_SIGNATURE = 0x06054b50;
    private static final int ZIP64_LOCATOR_SIGNATURE = 0x07064b50;
    private static final int ZIP64_END_SIGNATURE = 0x06064b50;
    private static final int CENTRAL_SIGNATURE = 0x02014b50;
    private static final int END_SIZE = 22;
    private static final int CENTRAL_HEADER_SIZE = 46;
    private static final int ZIP64_LOCATOR_SIZE = 20;
    private static final int ZIP64_END_MIN_SIZE = 56;
    private static final int MAX_UNSIGNED_SHORT = 0xffff;
    private static final long MAX_UNSIGNED_INT = 0xffffffffL;

    private ZipDirectoryPreflight() {}

    static Directory inspect(byte[] bytes, int maxEntries) {
        var archive = ByteBuffer.wrap(bytes).order(ByteOrder.LITTLE_ENDIAN);
        int last = archive.limit() - END_SIZE;
        int earliest = Math.max(0, last - MAX_UNSIGNED_SHORT);
        Directory directory = null;
        for (int candidate = last; candidate >= earliest; candidate--) {
            if (archive.getInt(candidate) != END_SIGNATURE
                    || unsignedShort(archive, candidate + 20) != last - candidate) {
                continue;
            }
            Directory candidateDirectory;
            try {
                candidateDirectory = inspectDirectory(archive, candidate, maxEntries);
            } catch (InvalidDirectoryException ignored) {
                // An opaque archive comment can contain a lookalike end record.
                continue;
            }
            require(directory == null);
            directory = candidateDirectory;
        }
        require(directory != null);
        return directory;
    }

    private static Directory inspectDirectory(ByteBuffer archive, int end, int maxEntries) {
        int disk = unsignedShort(archive, end + 4);
        int directoryDisk = unsignedShort(archive, end + 6);
        int diskEntries = unsignedShort(archive, end + 8);
        int entries = unsignedShort(archive, end + 10);
        long size = unsignedInt(archive, end + 12);
        long offset = unsignedInt(archive, end + 16);
        require(disk == 0 && directoryDisk == 0 && diskEntries == entries);

        int directoryEnd = end;
        int locator = end - ZIP64_LOCATOR_SIZE;
        long declaredEntries = entries;
        boolean hasLocatorSignature = locator >= 0 && archive.getInt(locator) == ZIP64_LOCATOR_SIGNATURE;
        boolean ordinaryEnvelope = offset <= end && size == end - offset;
        if (!ordinaryEnvelope && hasLocatorSignature) {
            require(archive.getInt(locator + 4) == 0 && archive.getInt(locator + 16) == 1);
            long zip64Offset = archive.getLong(locator + 8);
            require(zip64Offset >= 0 && zip64Offset <= locator - ZIP64_END_MIN_SIZE);
            directoryEnd = (int) zip64Offset;
            require(archive.getInt(directoryEnd) == ZIP64_END_SIGNATURE);
            require(archive.getLong(directoryEnd + 4) == locator - zip64Offset - 12);
            require(archive.getInt(directoryEnd + 16) == 0 && archive.getInt(directoryEnd + 20) == 0);
            declaredEntries = archive.getLong(directoryEnd + 32);
            require(archive.getLong(directoryEnd + 24) == declaredEntries);
            long zip64Size = archive.getLong(directoryEnd + 40);
            long zip64DirectoryOffset = archive.getLong(directoryEnd + 48);
            require(entries == MAX_UNSIGNED_SHORT || entries == declaredEntries);
            require(size == MAX_UNSIGNED_INT || size == zip64Size);
            require(offset == MAX_UNSIGNED_INT || offset == zip64DirectoryOffset);
            size = zip64Size;
            offset = zip64DirectoryOffset;
        }

        require(offset >= 0 && offset <= directoryEnd && size == directoryEnd - offset);
        int count = 0;
        for (int position = (int) offset; position < directoryEnd;) {
            require(directoryEnd - position >= CENTRAL_HEADER_SIZE
                    && archive.getInt(position) == CENTRAL_SIGNATURE);
            if (++count > maxEntries) {
                throw new IllegalArgumentException("Publication ZIP exceeds " + maxEntries + " entries");
            }
            int variableSize = unsignedShort(archive, position + 28)
                    + unsignedShort(archive, position + 30)
                    + unsignedShort(archive, position + 32);
            require(variableSize <= directoryEnd - position - CENTRAL_HEADER_SIZE);
            position += CENTRAL_HEADER_SIZE + variableSize;
        }
        require(count == declaredEntries);
        return new Directory(count, (int) offset, end + END_SIZE, ordinaryEnvelope && hasLocatorSignature);
    }

    private static void require(boolean valid) {
        if (!valid) {
            throw new InvalidDirectoryException();
        }
    }

    private static int unsignedShort(ByteBuffer archive, int offset) {
        return Short.toUnsignedInt(archive.getShort(offset));
    }

    private static long unsignedInt(ByteBuffer archive, int offset) {
        return Integer.toUnsignedLong(archive.getInt(offset));
    }

    private static final class InvalidDirectoryException extends IllegalArgumentException {
        InvalidDirectoryException() {
            super("Invalid publication ZIP central directory");
        }
    }

    record Directory(int entries, int startOffset, int endRecordLimit, boolean misleadingLocator) {
        byte[] readerArchive(byte[] archive) {
            if (misleadingLocator) {
                return withZip64End(archive);
            }
            if (endRecordLimit == archive.length) {
                return archive;
            }
            // The reader must see the same end record, even if the comment carries other signatures.
            byte[] view = Arrays.copyOf(archive, endRecordLimit);
            view[endRecordLimit - 2] = 0;
            view[endRecordLimit - 1] = 0;
            return view;
        }

        private byte[] withZip64End(byte[] archive) {
            // Keep the directory intact while giving the reader an unambiguous locator.
            int directoryEnd = endRecordLimit - END_SIZE;
            int directorySize = directoryEnd - startOffset;
            byte[] view = Arrays.copyOf(archive, endRecordLimit + ZIP64_END_MIN_SIZE + ZIP64_LOCATOR_SIZE);
            var end = ByteBuffer.wrap(view).order(ByteOrder.LITTLE_ENDIAN).position(directoryEnd);
            short version45 = 45;
            end.putInt(ZIP64_END_SIGNATURE).putLong(ZIP64_END_MIN_SIZE - 12)
                    .putShort(version45).putShort(version45).putInt(0).putInt(0)
                    .putLong(entries).putLong(entries).putLong(directorySize).putLong(startOffset);
            end.putInt(ZIP64_LOCATOR_SIGNATURE).putInt(0).putLong(directoryEnd).putInt(1);
            end.putInt(END_SIGNATURE).putInt(0).putShort((short) entries).putShort((short) entries)
                    .putInt(directorySize).putInt(startOffset).putShort((short) 0);
            return view;
        }
    }
}
