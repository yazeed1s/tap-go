package tap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
)

// CreateFileRequest uploads a document to Tap.
//
// File uploads are multipart, so this method is written by hand rather than
// generated.
type CreateFileRequest struct {
	// Purpose is what the file is for, e.g. identity_document,
	// business_logo, customer_signature, or dispute_evidence.
	Purpose string
	// Title is the document title, e.g. "Civil ID".
	Title string
	// FileName is the name sent with the file content.
	FileName string
	// File is the file content.
	File io.Reader
	// ExpiresAt is a future timestamp after which the file link stops
	// working.
	ExpiresAt string
	// FileLinkCreate creates a public link for the file.
	FileLinkCreate bool
}

// Create uploads a file.
//
// Tap docs: https://developers.tap.company/reference/create-a-file
func (s *FilesService) Create(
	ctx context.Context,
	req CreateFileRequest,
) (*File, error) {
	if req.File == nil {
		return nil, fmt.Errorf("tap: CreateFileRequest.File is required")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"purpose", req.Purpose},
		{"title", req.Title},
		{"expires_at", req.ExpiresAt},
		{"file_link_create", strconv.FormatBool(req.FileLinkCreate)},
	}
	for _, f := range fields {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return nil, fmt.Errorf("tap: encode file request: %w", err)
		}
	}
	part, err := w.CreateFormFile("file", req.FileName)
	if err != nil {
		return nil, fmt.Errorf("tap: encode file request: %w", err)
	}
	if _, err := io.Copy(part, req.File); err != nil {
		return nil, fmt.Errorf("tap: read file: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("tap: encode file request: %w", err)
	}

	httpReq, err := s.client.newRequest(ctx, http.MethodPost, "/v2/files", &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	body, err := s.client.send(httpReq)
	if err != nil {
		return nil, err
	}
	var out File
	if err := decodeJSON(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
