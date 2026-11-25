package publication

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

const (
	maxZipBytes      = 1_000_000
	maxExpandedBytes = 10_000_000
	maxZipEntries    = 1_000
	zipUTF8Names     = 1 << 11
	executionSource  = "loomspan/execution.yaml"
)

type PackReader interface {
	Read(digest string) ([]byte, error)
}

// MountedPacks reads immutable content-addressed ZIPs from a mounted directory.
type MountedPacks struct {
	root *os.Root
}

func OpenMountedPacks(dir string) (*MountedPacks, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errors.New("cannot open mounted publication storage")
	}
	return &MountedPacks{root: root}, nil
}

func (p *MountedPacks) Close() error {
	return p.root.Close()
}

func (p *MountedPacks) Read(digest string) ([]byte, error) {
	expected, err := hex.DecodeString(digest)
	if err != nil || len(expected) != sha256.Size || digest != strings.ToLower(digest) {
		return nil, errors.New("invalid publication ZIP digest")
	}
	file, err := p.root.Open(digest + ".zip")
	if err != nil {
		return nil, errors.New("cannot open mounted publication ZIP")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("mounted publication ZIP must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxZipBytes+1))
	if err != nil {
		return nil, errors.New("cannot read mounted publication ZIP")
	}
	if len(data) > maxZipBytes {
		return nil, errors.New("publication ZIP exceeds the maximum size")
	}
	actual := sha256.Sum256(data)
	if !bytes.Equal(actual[:], expected) {
		return nil, errors.New("publication ZIP digest does not match")
	}
	return data, nil
}

func scanPack(data []byte, namespace string) (variableDiscovery, []VariableIssue) {
	docs, err := executionDocuments(data, namespace)
	if err != nil {
		return variableDiscovery{}, []VariableIssue{{Reason: err.Error()}}
	}
	return discoverVariables(docs)
}

// executionDocuments reads authored inputs only. Java independently verifies the
// archive layout and budgets for every request, including companion entries.
func executionDocuments(data []byte, namespace string) ([]variableDocument, error) {
	if len(data) > maxZipBytes {
		return nil, errors.New("zip-size-limit")
	}
	view := zipDiscoveryView(data)
	archive, err := zip.NewReader(bytes.NewReader(view), int64(len(view)))
	if err != nil {
		return nil, errors.New("invalid-zip")
	}
	if len(archive.File) > maxZipEntries {
		return nil, errors.New("zip-entry-limit")
	}
	var configuration *variableDocument
	var skills []variableDocument
	remaining := int64(maxExpandedBytes)
	for _, file := range archive.File {
		source, inside := strings.CutPrefix(file.Name, namespace+"/")
		if !inside || !file.Mode().IsRegular() ||
			(source != executionSource && !(strings.HasPrefix(source, "loomspan/skills/") && strings.HasSuffix(source, ".yaml"))) {
			continue
		}
		if file.Flags&zipUTF8Names == 0 {
			// Match Java's ZIP decoder. The standard Go reader preserves the raw name bytes.
			source, err = charmap.CodePage437.NewDecoder().String(source)
			if err != nil {
				return nil, errors.New("invalid-zip-member-name")
			}
		}
		if file.UncompressedSize64 > uint64(remaining) {
			return nil, errors.New("expanded-zip-limit")
		}
		authored, err := readExecutionMember(file, remaining)
		if err != nil {
			return nil, err
		}
		remaining -= int64(len(authored))
		doc := variableDocument{source: source, authored: authored, execution: source == executionSource}
		if doc.execution {
			if configuration != nil {
				return nil, errors.New("duplicate-execution-configuration")
			}
			configuration = &doc
		} else {
			skills = append(skills, doc)
		}
	}
	if configuration == nil {
		return nil, errors.New("missing-execution-configuration")
	}
	return append([]variableDocument{*configuration}, skills...), nil
}

func readExecutionMember(file *zip.File, remaining int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, errors.New("invalid-zip-member")
	}
	defer reader.Close()
	authored, err := io.ReadAll(io.LimitReader(reader, remaining+1))
	if err != nil {
		return nil, errors.New("invalid-zip-member")
	}
	if int64(len(authored)) > remaining {
		return nil, errors.New("expanded-zip-limit")
	}
	return authored, nil
}
