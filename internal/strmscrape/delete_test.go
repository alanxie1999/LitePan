package strmscrape

import (
	"os"
	"path/filepath"
	"testing"

	"litepan/internal/strm"
)

func TestCollectCloudFileIDsFiltersAccountAndDupes(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "三体 (2023)")
	season := filepath.Join(show, "Season 01")
	mustMkdir(t, season)
	mustWrite(t, filepath.Join(season, "E01.strm"), strm.BuildPlayURL("http://127.0.0.1:5211", 7, "file-a", "E01.mkv", "token", false, nil))
	mustWrite(t, filepath.Join(season, "E02.strm"), strm.BuildPlayURL("http://127.0.0.1:5211", 7, "file-a", "E02.mkv", "token", false, nil))
	mustWrite(t, filepath.Join(season, "E03.strm"), strm.BuildPlayURL("http://127.0.0.1:5211", 9, "file-b", "E03.mkv", "token", false, nil))
	mustWrite(t, filepath.Join(season, "E04.strm"), "not-a-play-url\n")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	ids := collectCloudFileIDs(7, works[0])
	if len(ids) != 1 || ids[0] != "file-a" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestDeleteLocalWorkRemovesDirectoryAndKeepsSiblings(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "保留 (2024)")
	drop := filepath.Join(root, "删除 (2024)")
	mustMkdir(t, filepath.Join(keep, "Season 01"))
	mustMkdir(t, filepath.Join(drop, "Season 01"))
	mustWrite(t, filepath.Join(keep, "Season 01", "E01.strm"), "keep")
	mustWrite(t, filepath.Join(drop, "Season 01", "E01.strm"), "drop")
	mustWrite(t, filepath.Join(drop, "tvshow.nfo"), "<tvshow/>")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	var target workGroup
	for _, g := range works {
		if filepath.Base(g.absDir) == "删除 (2024)" {
			target = g
		}
	}
	if target.absDir == "" {
		t.Fatal("missing target work")
	}
	if err := deleteLocalWork(root, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("deleted dir still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(keep, "Season 01", "E01.strm")); err != nil {
		t.Fatalf("sibling should remain: %v", err)
	}
}

func TestDeleteLocalWorkRemovesFlatStrmAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	drop := filepath.Join(root, "删掉.strm")
	keep := filepath.Join(root, "留下.strm")
	mustWrite(t, drop, "x")
	mustWrite(t, keep, "y")
	mustWrite(t, filepath.Join(root, "删掉.nfo"), "<movie/>")
	mustWrite(t, filepath.Join(root, "删掉-poster.jpg"), "img")
	mustWrite(t, filepath.Join(root, "留下.nfo"), "<movie/>")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	var target workGroup
	for _, g := range works {
		if g.flatFile == drop {
			target = g
		}
	}
	if target.flatFile == "" {
		t.Fatal("missing flat work")
	}
	if err := deleteLocalWork(root, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("flat strm still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "删掉.nfo")); !os.IsNotExist(err) {
		t.Fatalf("flat nfo still exists: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("sibling strm should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "留下.nfo")); err != nil {
		t.Fatalf("sibling nfo should remain: %v", err)
	}
}

func TestDeleteLocalWorkRejectsLibraryRoot(t *testing.T) {
	root := t.TempDir()
	err := deleteLocalWork(root, workGroup{absDir: root})
	if err == nil {
		t.Fatal("expected reject deleting library root")
	}
}
