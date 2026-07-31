package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kaidstor/finances-next/cli/internal/output"
)

// Соответствие повторяет ALLOWED_EXTENSIONS и ALLOWED_MIME_TYPES из
// lib/services/file-storage.ts. Тип определяем сами, а не через
// mime.TypeByExtension: тот зависит от таблиц системы и для .docx/.xlsx
// возвращает не то, что ждёт сервер. Заодно отсекаем лишнее до отправки,
// чтобы не гонять мегабайты ради 400 в ответ.
var uploadTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".pdf":  "application/pdf",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

const maxFileSize = 10 << 20

const attachHelp = `finances-kai attach <id> <файл>… — приложить файлы к платежу

Вложения попадают в архив, который отдаёт export.
Принимаются jpg, png, gif, webp, pdf, txt, csv, doc(x), xls(x); до 10 МБ.

  finances-kai attach 11111111 ~/Downloads/чек.pdf
  finances-kai attach 11111111 скрин1.png скрин2.png
  finances-kai attach 11111111 --list
  finances-kai attach 11111111 --rm <id-файла>`

func cmdAttach(ctx context.Context, args []string) error {
	fs, profile, asJSON := newFlagSet("attach", attachHelp)
	list := fs.Bool("list", false, "показать вложения платежа")
	remove := fs.String("rm", "", "удалить вложение по id")

	words, flags := splitLeading(args)
	if err := fs.Parse(flags); err != nil {
		return errParsed
	}
	words = append(words, fs.Args()...)
	if len(words) == 0 {
		fs.Usage()
		return errParsed
	}

	paymentID, paths := words[0], words[1:]

	s, err := open(*profile)
	if err != nil {
		return err
	}
	p, err := s.findPayment(ctx, paymentID)
	if err != nil {
		return err
	}

	switch {
	case *remove != "":
		if err := s.client.DeletePaymentFile(ctx, p.ID, *remove); err != nil {
			return err
		}
		fmt.Println(output.Yellow("вложение удалено"), *remove)
		return nil

	case *list || len(paths) == 0:
		files, err := s.client.PaymentFiles(ctx, p.ID)
		if err != nil {
			return err
		}
		if *asJSON {
			return output.JSON(files)
		}
		if len(files) == 0 {
			fmt.Printf("У платежа %s вложений нет\n", describe(p))
			return nil
		}
		t := output.NewTable("ФАЙЛ", "РАЗМЕР", "ТИП", "ID").RightAlign(1)
		for _, f := range files {
			t.Add(f.OriginalName, humanSize(int(f.Size)), f.MimeType, output.Dim(shortID(f.ID)))
		}
		t.Render(os.Stdout)
		return nil
	}

	// Проверяем все файлы до первой отправки: иначе часть уедет, команда упадёт
	// на середине, и придётся выяснять, что успело загрузиться.
	types := make([]string, len(paths))
	for i, path := range paths {
		contentType, err := checkUploadable(path)
		if err != nil {
			return err
		}
		types[i] = contentType
	}

	for i, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("читаю %s: %w", path, err)
		}
		file, err := s.client.UploadFile(ctx, p.ID, filepath.Base(path), types[i], content)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s · %s %s\n", output.Green("приложен"), file.OriginalName,
			humanSize(int(file.Size)), output.Dim(shortID(file.ID)))
	}
	fmt.Printf("%s\n", output.Dim("к платежу: "+describe(p)+" от "+output.Date(p.Date)))
	return nil
}

// checkUploadable проверяет файл и заодно отдаёт MIME-тип для отправки.
func checkUploadable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("файл %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s — каталог, а не файл", path)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("%s весит %s, максимум 10 МБ", path, humanSize(int(info.Size())))
	}
	ext := strings.ToLower(filepath.Ext(path))
	contentType, ok := uploadTypes[ext]
	if !ok {
		return "", fmt.Errorf("%s: расширение %q сервер не принимает; можно jpg, png, gif, webp, pdf, txt, csv, doc(x), xls(x)",
			path, ext)
	}
	return contentType, nil
}
