package strmscrape

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"litepan/internal/mediaorganize/rules"
)

func TestPickTMDBScrapeMatchUsesControlledAdjacentYearDoubt(t *testing.T) {
	year := 2026
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"id":2025,"title":"测试电影","release_date":"2025-12-20"}`),
	})
	selected, doubt := pickTMDBScrapeMatch(results, &year, MediaTypeMovie, "测试电影")
	if id, _, _, _ := rules.ExtractTMDBDisplayFields(selected, MediaTypeMovie); id != "2025" || !doubt {
		t.Fatalf("唯一强同名 ±1 年候选应命中并标记存疑，id=%q doubt=%v", id, doubt)
	}
}

func TestPickTMDBScrapeMatchPrefersExactYear(t *testing.T) {
	year := 2026
	results := rules.RawJSONListToMaps([]json.RawMessage{
		json.RawMessage(`{"id":2025,"title":"测试电影","release_date":"2025-01-01"}`),
		json.RawMessage(`{"id":2026,"title":"测试电影","release_date":"2026-01-01"}`),
	})
	selected, doubt := pickTMDBScrapeMatch(results, &year, MediaTypeMovie, "测试电影")
	if id, _, _, _ := rules.ExtractTMDBDisplayFields(selected, MediaTypeMovie); id != "2026" || doubt {
		t.Fatalf("完全相等年份必须优先且不存疑，id=%q doubt=%v", id, doubt)
	}
}

func TestDecodeTMDBInfoReadsVoteAverage(t *testing.T) {
	raw := json.RawMessage(`{"id":969681,"title":"Spider-Man: Brand New Day","overview":"plot","poster_path":"/p.jpg","vote_average":7.4,"release_date":"2026-07-29"}`)
	info, err := decodeTMDBInfo(raw, MediaTypeMovie)
	if err != nil {
		t.Fatal(err)
	}
	if info.Rating != 7.4 {
		t.Fatalf("应读取 TMDB vote_average，got=%v", info.Rating)
	}
}

func TestWriteMovieNFOIncludesRating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "movie.nfo")
	year := 2026
	if err := writeMovieNFO(path, "蜘蛛侠：崭新之日", "969681", "剧情", &year, 7.4); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "<rating>7.4</rating>") {
		t.Fatalf("NFO 应包含用户评分，got=%s", body)
	}
}
