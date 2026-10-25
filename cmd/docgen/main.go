// docgen пишет синтетические документы и эталоны к ним. Без -inn собирает
// набор для testdata/documents: демо-компании, две из генератора, ловушку,
// один скан в PDF и образцы паспортов руководителей демо-компаний.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
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
	if *inn == "" {
		passports(*out, filePerm)
	}
	log.Printf("%d документов в %s", len(docs), *out)
}

// passports: ФИО берём из выписки ЕГРЮЛ мока, чтобы паспорт сходился с компанией.
// У ИП руководителя в выписке нет, он сам и есть компания.
func passports(out string, perm os.FileMode) {
	ctx := context.Background()
	egrul := mocks.New()
	for _, inn := range []string{"0000100012", "000020001010", "0000300011", "0000400016", "0000500010", "0000700010"} {
		rec := httptest.NewRecorder()
		egrul.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/egrul/"+inn, http.NoBody))
		var r struct {
			FullName string `json:"full_name"`
			Director *struct {
				FIO string `json:"fio"`
			} `json:"director"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
			log.Fatalf("егрюл %s: %v", inn, err)
		}
		fio := strings.TrimPrefix(r.FullName, "Индивидуальный предприниматель ")
		if r.Director != nil {
			fio = r.Director.FIO
		}
		file, err := docgen.PassportPDF(fio, inn)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, inn+"_passport.pdf"), file, perm); err != nil {
			log.Fatal(err)
		}
	}
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
