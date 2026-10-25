package docgen

import "strings"

// PassportPDF рисует разворот паспорта для загрузки в заявку. Паспорт мы не распознаём,
// банку важен сам файл, поэтому данные условные, кроме ФИО, а поверх крупно «ОБРАЗЕЦ».
// Серия 00 00 не выдаётся, с настоящим паспортом номер не совпадёт.
func PassportPDF(fio, inn string) ([]byte, error) {
	f := newPDF()
	f.AddUTF8FontFromBytes("pt", "", regularTTF)
	f.AddUTF8FontFromBytes("pt", "B", boldTTF)
	f.AddPage()

	// водяной знак первым, текст поверх него остаётся чёрным
	f.SetFont("pt", "B", 72)
	f.SetTextColor(200, 200, 200)
	f.TransformBegin()
	f.TransformRotate(30, 105, 80)
	f.Text(47, 90, "ОБРАЗЕЦ")
	f.TransformEnd()
	f.SetTextColor(0, 0, 0)

	f.SetFont("pt", "B", 13)
	f.SetXY(left, 20)
	f.CellFormat(right-left, 8, "ПАСПОРТ ГРАЖДАНИНА РОССИЙСКОЙ ФЕДЕРАЦИИ", "", 1, "C", false, 0, "")
	f.Rect(left, 32, right-left, 110, "D")
	// место под фото
	f.Rect(left+6, 40, 35, 45, "D")

	name := strings.Fields(fio)
	for len(name) < 3 {
		name = append(name, "")
	}
	sex := "МУЖ."
	if strings.HasSuffix(name[2], "вна") {
		sex = "ЖЕН."
	}
	rows := [][2]string{
		{"Фамилия", name[0]},
		{"Имя", name[1]},
		{"Отчество", name[2]},
		{"Пол", sex},
		{"Дата рождения", "01.01.1985"},
		{"Место рождения", "г. Москва"},
		{"Серия и номер", "00 00 " + inn[len(inn)-6:]},
		{"Выдан", "ГУ МВД России по г. Москве"},
		{"Дата выдачи", "15.06.2015"},
	}
	y := 46.0
	for _, r := range rows {
		f.SetFont("pt", "", 9)
		f.Text(left+48, y, r[0])
		f.SetFont("pt", "B", 11)
		f.Text(left+85, y, r[1])
		y += 10
	}

	f.SetFont("pt", "", 7.5)
	f.Text(left, 285, "Прототип Alfa-Hack 2026, синтетический документ, не удостоверяет личность.")
	return output(f)
}
