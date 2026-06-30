package formatter_test

import (
	"os"
	"testing"

	"github.com/heptau/pg_procrustes/internal/config"
	"github.com/heptau/pg_procrustes/internal/formatter"
)

func loadBenchFixture(b *testing.B, dir string) (string, *config.Config) {
	b.Helper()
	src, err := os.ReadFile("testdata/" + dir + "/input.sql")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	cfg, err := config.Load("testdata/" + dir + "/config.yaml")
	if err != nil {
		b.Fatalf("load config: %v", err)
	}
	return string(src), cfg
}

func BenchmarkFormatKeywordsCasing(b *testing.B) {
	src, cfg := loadBenchFixture(b, "keywords_casing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(src, cfg)
	}
}

func BenchmarkFormatClauseBreak(b *testing.B) {
	src, cfg := loadBenchFixture(b, "clause_break")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(src, cfg)
	}
}

func BenchmarkFormatDDL(b *testing.B) {
	src, cfg := loadBenchFixture(b, "ddl_full")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(src, cfg)
	}
}

func BenchmarkFormatDML(b *testing.B) {
	src, cfg := loadBenchFixture(b, "dml_full")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(src, cfg)
	}
}

func BenchmarkFormatCTEWindow(b *testing.B) {
	src, cfg := loadBenchFixture(b, "cte_window")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(src, cfg)
	}
}

func BenchmarkFormatPLpgSQLFull(b *testing.B) {
	src, cfg := loadBenchFixture(b, "plpgsql_full")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(src, cfg)
	}
}
