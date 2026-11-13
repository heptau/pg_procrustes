package formatter_test

import (
	"testing"

	"github.com/heptau/pg_procrustes/config"
	"github.com/heptau/pg_procrustes/formatter"
)

// PostgreSQL folds only A–Z in an unquoted name (UTF-8 database): ü and Ü are two identifiers,
// so no case rule may change the other letters.
func TestIdentifierCaseChangesAsciiLettersOnly(t *testing.T) {
	for rule, want := range map[config.CaseRule]string{
		config.CaseUpper:      "select äö, AB, Μ\nfrom ÜNï, éA;\n",
		config.CaseLower:      "select äö, ab, Μ\nfrom Ünï, éa;\n",
		config.CaseCapitalize: "select äö, Ab, Μ\nfrom Ünï, éa;\n",
	} {
		c := verbatimCfg(t)
		c.Columns.Case = rule
		c.Tables.Case = rule
		got, err := formatter.Format("select äö, aB, Μ from Ünï, éa;\n", c)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s:\n got %q\nwant %q", rule, got, want)
		}
	}
}
