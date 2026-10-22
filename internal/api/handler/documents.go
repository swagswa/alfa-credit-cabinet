package handler

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (h *Handler) UploadDocument(
	ctx context.Context,
	req oapi.UploadDocumentRequestObject,
) (oapi.UploadDocumentResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	kind, name, file, err := readUpload(req.Body)
	if err != nil {
		return nil, err
	}
	d, err := h.docs.Upload(ctx, a, req.Id, domain.DocKind(kind), name, file)
	if err != nil {
		return nil, err
	}
	return oapi.UploadDocument201JSONResponse(toDocument(&d)), nil
}

func (h *Handler) GetDocument(
	ctx context.Context,
	req oapi.GetDocumentRequestObject,
) (oapi.GetDocumentResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.docs.Get(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	return oapi.GetDocument200JSONResponse(toDocument(&d)), nil
}

func (h *Handler) DownloadDocument(
	ctx context.Context,
	req oapi.DownloadDocumentRequestObject,
) (oapi.DownloadDocumentResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	f, err := h.docs.File(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	body, size := bytes.NewReader(f.Content), int64(len(f.Content))
	hdr := oapi.DownloadDocument200ResponseHeaders{
		ContentDisposition: new(mime.FormatMediaType("attachment", map[string]string{"filename": f.FileName})),
	}
	// тип в базе только из этих трёх, CHECK в схеме
	switch f.ContentType {
	case "image/png":
		return oapi.DownloadDocument200ImagepngResponse{Body: body, Headers: hdr, ContentLength: size}, nil
	case "image/jpeg":
		return oapi.DownloadDocument200ImagejpegResponse{Body: body, Headers: hdr, ContentLength: size}, nil
	}
	return oapi.DownloadDocument200ApplicationpdfResponse{Body: body, Headers: hdr, ContentLength: size}, nil
}

func (h *Handler) ConfirmDocument(
	ctx context.Context,
	req oapi.ConfirmDocumentRequestObject,
) (oapi.ConfirmDocumentResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.docs.Confirm(ctx, a, req.Id, *recode[domain.DocValues](req.Body))
	if err != nil {
		return nil, err
	}
	return oapi.ConfirmDocument200JSONResponse(toDocument(&d)), nil
}

// readUpload разбирает форму сам: файл в памяти, без временных файлов ReadForm
func readUpload(r *multipart.Reader) (kind, name string, file []byte, err error) {
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			return kind, name, file, nil
		}
		if err != nil {
			return "", "", nil, fmt.Errorf("%w: %w", domain.ErrInvalid, err)
		}
		// на байт больше лимита: так usecase отличит ровно 10 МБ от большего файла
		b, err := io.ReadAll(io.LimitReader(p, domain.MaxDocSize+1))
		if err != nil {
			return "", "", nil, fmt.Errorf("%w: %w", domain.ErrInvalid, err)
		}
		switch p.FormName() {
		case "kind":
			kind = string(b)
		case "file":
			name, file = p.FileName(), b
		}
	}
}

func toDocument(d *domain.Document) oapi.Document {
	v := oapi.Document{
		Id:               d.ID,
		ApplicationId:    d.ApplicationID,
		Kind:             oapi.DocumentKind(d.Kind),
		Status:           oapi.DocumentStatus(d.Status),
		FileName:         d.FileName,
		ContentType:      oapi.DocumentContentType(d.ContentType),
		SizeBytes:        int64(d.Size),
		Sha256:           hex.EncodeToString(d.SHA256),
		TextSource:       opt(oapi.DocumentTextSource(d.TextSource)),
		Extracted:        recode[oapi.DocumentValues](d.Extracted),
		Confirmed:        recode[oapi.DocumentValues](d.Confirmed),
		ValidationErrors: recode[[]oapi.FieldError](d.Problems),
		Error:            opt(d.Error),
		Model:            opt(d.Model),
		CreatedAt:        d.CreatedAt,
		ConfirmedAt:      d.ConfirmedAt,
	}
	if len(d.EditedFields) > 0 {
		v.EditedFields = &d.EditedFields
	}
	return v
}

// recode перекладывает итоги документа между oapi и domain. JSON у них один и тот
// же (см. domain.Statement), поле за полем переписывать незачем. nil остаётся nil.
func recode[T any](v any) (out *T) {
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &out)
	return out
}
