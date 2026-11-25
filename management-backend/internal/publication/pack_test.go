package publication

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type testZipMember struct {
	name string
	data []byte
}

func testPack(t *testing.T, method uint16, members ...testZipMember) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, item := range members {
		member, err := writer.CreateHeader(&zip.FileHeader{Name: item.name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(item.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func testWritePack(t *testing.T, dir string, data []byte) string {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if err := os.WriteFile(filepath.Join(dir, digest+".zip"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestMountedPack(t *testing.T) {
	dir := t.TempDir()
	packs, err := OpenMountedPacks(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer packs.Close()

	execution := testZipMember{"sales/loomspan/execution.yaml", []byte("loomspan: {}")}
	skill := testZipMember{"sales/loomspan/skills/nested/read.yaml", []byte("executor: demo\nconfig: {path: {$var: lookup}}\n")}
	companion := testZipMember{"sales/notes.yaml", []byte("ignored: {$var: ignored}")}
	original := testPack(t, zip.Deflate, skill, companion, execution)
	digest := testWritePack(t, dir, original)
	data, err := packs.Read(digest)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("mounted pack did not return original bytes: %v", err)
	}
	found, issues := scanPack(data, "sales")
	want := []VariableOccurrence{{"lookup", "loomspan/skills/nested/read.yaml", "config.path"}}
	if len(issues) != 0 || !slices.Equal(found.Occurrences, want) {
		t.Fatalf("got %+v, %v", found, issues)
	}
	for _, invalid := range []string{"../" + digest, digest + ".zip", "nothex"} {
		if _, err := packs.Read(invalid); err == nil {
			t.Fatal("invalid mounted path accepted")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, digest+".zip"), []byte("new data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := packs.Read(digest); err == nil {
		t.Fatal("changed content accepted")
	}

	for _, extra := range []int{0, 1} {
		companion.data = nil
		headerSize := len(testPack(t, zip.Store, execution, companion))
		companion.data = make([]byte, maxZipBytes-headerSize+extra)
		pack := testPack(t, zip.Store, execution, companion)
		if len(pack) != maxZipBytes+extra {
			t.Fatalf("unexpected ZIP test size %d", len(pack))
		}
		_, err := packs.Read(testWritePack(t, dir, pack))
		if (err != nil) != (extra != 0) {
			t.Fatalf("ZIP size=%d, error=%v", len(pack), err)
		}

		skill.data = make([]byte, maxExpandedBytes-len(execution.data)+extra)
		pack = testPack(t, zip.Deflate, execution, skill)
		_, err = executionDocuments(pack, "sales")
		if (err != nil) != (extra != 0) {
			t.Fatalf("expanded size=%d, error=%v", len(execution.data)+len(skill.data), err)
		}
	}
}

func TestMountedPackDiscoversVariablesWithAnOpaqueZIPComment(t *testing.T) {
	pack := testPack(t, zip.Deflate,
		testZipMember{"sales/loomspan/execution.yaml", []byte("loomspan: {}")},
		testZipMember{"sales/loomspan/skills/read.yaml", []byte("executor: demo\nconfig: {path: {$var: lookup}}\n")},
	)
	const commentSize = 60
	end := len(pack) - 22
	pack = append(pack, make([]byte, commentSize)...)
	binary.LittleEndian.PutUint16(pack[end+20:], commentSize)
	lookalike := end + 22 + 7
	copy(pack[lookalike:], "PK\x05\x06")
	binary.LittleEndian.PutUint16(pack[lookalike+20:], uint16(len(pack)-lookalike-22))
	original := bytes.Clone(pack)

	found, issues := scanPack(pack, "sales")
	want := []VariableOccurrence{{"lookup", "loomspan/skills/read.yaml", "config.path"}}
	if len(issues) != 0 || !slices.Equal(found.Occurrences, want) || !bytes.Equal(pack, original) {
		t.Fatalf("got %+v, %v; original bytes preserved: %t", found, issues, bytes.Equal(pack, original))
	}
}

func TestMountedPackDiscoversVariablesWithLocatorInEntryComment(t *testing.T) {
	const name = "sales/" + executionSource
	comment := make([]byte, 0xffff-46-len(name))
	metadata := comment[len(comment)-20:]
	copy(metadata, "PK\x06\x07")
	binary.LittleEndian.PutUint32(metadata[16:], 1)
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Comment: string(comment), Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("loomspan: {connections: {model: {key: {$var: lookup}}}}")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := archive.Bytes()
	if size := binary.LittleEndian.Uint32(data[len(data)-22+12:]); size != 0xffff {
		t.Fatalf("unexpected fixture directory size: %d", size)
	}
	original := bytes.Clone(data)
	found, issues := scanPack(data, "sales")
	want := []VariableOccurrence{{"lookup", executionSource, "loomspan.connections.model.key"}}
	if len(issues) != 0 || !slices.Equal(found.Occurrences, want) || !bytes.Equal(data, original) {
		t.Fatalf("got %+v, %v; original bytes preserved: %t", found, issues, bytes.Equal(data, original))
	}
}

func TestMountedPackPreservesLegacyZIPSourceName(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	configuration, err := writer.Create("sales/" + executionSource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configuration.Write([]byte("loomspan: {}")); err != nil {
		t.Fatal(err)
	}
	skill, err := writer.CreateHeader(&zip.FileHeader{
		Name: "sales/loomspan/skills/caf\x82.yaml", NonUTF8: true, Method: zip.Deflate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skill.Write([]byte("executor: demo\nconfig: {path: {$var: lookup}}\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	found, issues := scanPack(archive.Bytes(), "sales")
	_, blockers := variableValues(found, nil)
	want := []VariableIssue{{"missing-value", "loomspan/skills/café.yaml", "config.path"}}
	if len(issues) != 0 || !slices.Equal(blockers, want) {
		t.Fatalf("got blockers: %#v, pack issues: %v", blockers, issues)
	}
}

func TestMountedPackDiscoversVariablesWithZIP64DirectorySize(t *testing.T) {
	pack := testPack(t, zip.Deflate,
		testZipMember{"sales/loomspan/execution.yaml", []byte("loomspan: {}")},
		testZipMember{"sales/loomspan/skills/read.yaml", []byte("executor: demo\nconfig: {path: {$var: lookup}}\n")},
	)
	end := len(pack) - zipEndSize
	widened := zip64DiscoveryView(pack[:end], pack[end:])
	// The ordinary end record can retain the entry count and offset while
	// taking only its directory size from the actual ZIP64 end record.
	binary.LittleEndian.PutUint32(widened[len(widened)-zipEndSize+12:], 0xffffffff)
	original := bytes.Clone(widened)

	found, issues := scanPack(widened, "sales")
	want := []VariableOccurrence{{"lookup", "loomspan/skills/read.yaml", "config.path"}}
	if len(issues) != 0 || !slices.Equal(found.Occurrences, want) || !bytes.Equal(widened, original) {
		t.Fatalf("got %+v, %v; original bytes preserved: %t", found, issues, bytes.Equal(widened, original))
	}
}
