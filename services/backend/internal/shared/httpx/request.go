package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/AskarKasimov/ai-tutor/services/backend/internal/shared/fault"
)

func DecodeJSON(r *http.Request, dst any) error {
	if !strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]), "application/json") {
		return fault.Validation("body", "Ожидается application/json.")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return BodyError(err)
	}
	if !utf8.Valid(body) {
		return fault.Validation("body", "Ожидается UTF-8.")
	}
	d := json.NewDecoder(strings.NewReader(string(body)))
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return fault.Validation("body", "Некорректный JSON или лишние поля.")
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return fault.Validation("body", "Ожидается ровно один объект JSON.")
	}
	return nil
}

func BodyError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Превышен допустимый размер запроса.")
	}
	return fault.Validation("body", "Не удалось прочитать запрос.")
}

func NoBody(r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return BodyError(err)
	}
	if len(body) > 0 {
		return fault.Validation("body", "Тело запроса не допускается.")
	}
	return nil
}

func Cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil || len(c.Value) > 128 {
		return ""
	}
	return c.Value
}

func Upload(r *http.Request, field string, maxBytes int64) ([]byte, string, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, "", fault.Validation(field, "Ожидается multipart/form-data с одним файлом.")
	}
	part, err := reader.NextPart()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, "", fault.Validation(field, "Файл обязателен.")
		}
		return nil, "", BodyError(err)
	}
	defer part.Close()
	if part.FormName() != field || part.FileName() == "" {
		return nil, "", fault.Validation(field, "Ожидается ровно одно поле-файл "+field+".")
	}
	media, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil {
		media = ""
	}
	data, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
	if err != nil {
		return nil, "", BodyError(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fault.New(fault.TooLarge, "UPLOAD_TOO_LARGE", "Файл превышает 25 МиБ.")
	}
	extra, err := reader.NextPart()
	if err != io.EOF {
		if extra != nil {
			extra.Close()
		}
		if err != nil {
			return nil, "", BodyError(err)
		}
		return nil, "", fault.Validation(field, "Лишние или повторные multipart-поля запрещены.")
	}
	return data, mime.FormatMediaType(strings.ToLower(media), params), nil
}
