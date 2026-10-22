// docgen пишет синтетические документы и эталоны к ним. Без -inn собирает
// набор для testdata/documents: демо-компании, две из генератора, ловушку
// и один скан в PDF.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
)

func main() {
	inn := flag.String("inn", "", "ИНН; без него весь набор")
	kind := flag.String("kind", docgen.KindStatement, "statement или financials")
	format := flag.String("format", "a", "a, b (текстовые PDF) или c (скан PNG)")
	trap := flag.Bool("trap", false, "завысить приход в 1,5 раза")
	asPDF := flag.Bool("pdf", false, "скан завернуть в PDF")
	out := flag.String("out", "testdata/documents", "каталог")
	flag.Parse()

	docs := []docgen.Doc{{INN: *inn, Kind: *kind, Format: *format, Trap: *trap, PDF: *asPDF}}
	if *inn == "" {
		docs = set()
	}
	const dirPerm, filePerm = 0o750, 0o600
	if err := os.MkdirAll(*out, dirPerm); err != nil {
		log.Fatal(err)
	}
	for _, d := range docs {
		file, truth, err := docgen.Generate(d)
		if err != nil {
			log.Fatalf("%s: %v", d.Name(), err)
		}
		path := filepath.Join(*out, d.Name())
		if err := os.WriteFile(path+d.Ext(), file, filePerm); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(path+".truth.json", truth, filePerm); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("%d документов в %s", len(docs), *out)
}

func set() []docgen.Doc {
	// 0012345673 и 001234567887 из генератора: регион 00 ФНС не выдаёт, с реальной компанией не совпадут
	orgs := []string{"0000100012", "0000300011", "0000400016", "0012345673"}
	ips := []string{"000020001010", "001234567887"}
	var docs []docgen.Doc
	for _, inn := range append(orgs, ips...) {
		for _, f := range []string{"a", "b", "c"} {
			docs = append(docs, docgen.Doc{INN: inn, Kind: docgen.KindStatement, Format: f})
		}
	}
	// у ИП вместо отчётности декларация, её не распознаём
	for _, inn := range orgs {
		docs = append(docs, docgen.Doc{INN: inn, Kind: docgen.KindFinancials, Format: "a"})
	}
	return append(docs,
		docgen.Doc{INN: "0000300011", Kind: docgen.KindStatement, Format: "a", Trap: true},
		docgen.Doc{INN: "0000100012", Kind: docgen.KindStatement, Format: "c", PDF: true},
	)
}
