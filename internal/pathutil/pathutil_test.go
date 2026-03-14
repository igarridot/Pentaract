package pathutil

import "testing"

func TestJoin(t *testing.T) {
	tests := []struct {
		name string
		base string
		part string
		want string
	}{
		{name: "joins normalized segments", base: "/root/docs/", part: "/file.txt/", want: "root/docs/file.txt"},
		{name: "returns part when base empty", base: "", part: "/file.txt/", want: "file.txt"},
		{name: "returns base when part empty", base: "/root/docs/", part: "", want: "root/docs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Join(tt.base, tt.part); got != tt.want {
				t.Fatalf("Join(%q, %q) = %q, want %q", tt.base, tt.part, got, tt.want)
			}
		})
	}
}

func TestTrimTrailingSlash(t *testing.T) {
	if got := TrimTrailingSlash("root/docs///"); got != "root/docs" {
		t.Fatalf("TrimTrailingSlash returned %q", got)
	}
}

func TestArchiveName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "", want: "files"},
		{path: "/", want: "files"},
		{path: "root/docs", want: "docs"},
		{path: "root/docs/", want: "docs"},
		{path: "docs", want: "docs"},
	}

	for _, tt := range tests {
		if got := ArchiveName(tt.path); got != tt.want {
			t.Fatalf("ArchiveName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestSplitDirAndFile(t *testing.T) {
	tests := []struct {
		path         string
		wantDir      string
		wantFilename string
	}{
		{path: "dir/video.mkv", wantDir: "dir/", wantFilename: "video.mkv"},
		{path: "video.mkv", wantDir: "", wantFilename: "video.mkv"},
		{path: "dir/sub/", wantDir: "dir/sub/", wantFilename: ""},
	}

	for _, tt := range tests {
		dir, filename := SplitDirAndFile(tt.path)
		if dir != tt.wantDir || filename != tt.wantFilename {
			t.Fatalf("SplitDirAndFile(%q) = (%q, %q), want (%q, %q)", tt.path, dir, filename, tt.wantDir, tt.wantFilename)
		}
	}
}

func TestSplitNameAndExtension(t *testing.T) {
	tests := []struct {
		filename string
		wantName string
		wantExt  string
	}{
		{filename: "video.mkv", wantName: "video", wantExt: ".mkv"},
		{filename: "archive.tar.gz", wantName: "archive.tar", wantExt: ".gz"},
		{filename: "README", wantName: "README", wantExt: ""},
		{filename: ".env", wantName: ".env", wantExt: ""},
	}

	for _, tt := range tests {
		name, ext := SplitNameAndExtension(tt.filename)
		if name != tt.wantName || ext != tt.wantExt {
			t.Fatalf("SplitNameAndExtension(%q) = (%q, %q), want (%q, %q)", tt.filename, name, ext, tt.wantName, tt.wantExt)
		}
	}
}
