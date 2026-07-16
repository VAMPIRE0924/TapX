package panel

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestBuildUpdateVersions(t *testing.T) {
	versions := buildUpdateVersions("1.2.0", []string{"v1.3.0", "v1.2.0", "v1.1.0"}, true)
	if len(versions) != 3 {
		t.Fatalf("version count = %d, want 3: %+v", len(versions), versions)
	}
	if !versions[0].Current || versions[0].Version != "1.2.0" {
		t.Fatalf("current version = %+v", versions[0])
	}
	if !versions[1].Latest || !versions[1].Installable || versions[1].Version != "1.3.0" {
		t.Fatalf("latest version = %+v", versions[1])
	}
}

func TestBuildUpdateVersionsDisablesBundledComponents(t *testing.T) {
	versions := buildUpdateVersions("0.1.0", []string{"v0.2.0"}, false)
	for _, version := range versions {
		if version.Installable {
			t.Fatalf("bundled version unexpectedly installable: %+v", version)
		}
	}
}

func TestOfficialXrayAsset(t *testing.T) {
	if got := officialXrayAsset("linux", "amd64"); got != "Xray-linux-64.zip" {
		t.Fatalf("linux amd64 asset = %q", got)
	}
	if got := officialXrayAsset("plan9", "amd64"); got != "" {
		t.Fatalf("unsupported asset = %q", got)
	}
}

func TestExtractTapXBinaries(t *testing.T) {
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, body := range map[string]string{
		"tapx-linux-amd64/tapx-panel": "panel-binary",
		"tapx-linux-amd64/tapx-core":  "core-binary",
		"tapx-linux-amd64/install.sh": "ignored",
	} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	binaries, err := extractTapXBinaries(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(binaries["tapx-panel"]) != "panel-binary" || string(binaries["tapx-core"]) != "core-binary" {
		t.Fatalf("unexpected extracted binaries: %+v", binaries)
	}
}
