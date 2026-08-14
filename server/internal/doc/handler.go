package doc

import (
	"path/filepath"
	"strconv"
	"strings"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// allowedExtensions defines the file types accepted for upload. Matches the
// parsers supported by the local parser and MinerU.
var allowedExtensions = map[string]bool{
	".txt": true, ".md": true, ".markdown": true,
	".pdf": true, ".docx": true,
}

// Upload handles multipart/form-data upload. Field name: "file".
// Optional form fields: kb_id (path param), name (override).
func (h *Handler) Upload(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	if kbID == "" {
		return response.Fail(c, errs.BadRequest("kb_id is required"))
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return response.Fail(c, errs.BadRequest("missing file field: "+err.Error()))
	}
	if fileHeader.Size > 50*1024*1024 {
		return response.Fail(c, errs.BadRequest("file too large (max 50MB)"))
	}
	src, err := fileHeader.Open()
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "open upload", err))
	}
	defer src.Close()

	name := c.FormValue("name")
	if name == "" {
		name = fileHeader.Filename
	}

	ext := strings.ToLower(filepath.Ext(name))
	if !allowedExtensions[ext] {
		return response.Fail(c, errs.BadRequest("unsupported file type: "+ext))
	}

	doc, err := h.svc.Upload(c.Context(),
		middleware.TenantID(c),
		middleware.UserID(c),
		kbID,
		name,
		fileHeader.Header.Get("Content-Type"),
		fileHeader.Size,
		src,
	)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.Created(c, doc)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	doc, err := h.svc.Get(c.Context(), middleware.TenantID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, doc)
}

// Content returns the parsed document content (markdown/text) for the document
// viewer. Used by citation tracing to display the source document.
func (h *Handler) Content(c *fiber.Ctx) error {
	doc, content, err := h.svc.GetContent(c.Context(), middleware.TenantID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{
		"name":         doc.Name,
		"mime_type":    doc.MimeType,
		"status":       doc.Status,
		"chunk_count":  doc.ChunkCount,
		"content":      content,
	})
}

func (h *Handler) List(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "20"))
	items, total, err := h.svc.List(c.Context(), middleware.TenantID(c), kbID, page, size)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func (h *Handler) ListChunks(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "200"))
	chunks, err := h.svc.ListChunks(c.Context(), middleware.TenantID(c), c.Params("id"), page, size)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": chunks, "page": page, "size": size})
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(c.Context(), middleware.TenantID(c), c.Params("id")); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}

func (h *Handler) Reparse(c *fiber.Ctx) error {
	if err := h.svc.Reparse(c.Context(), middleware.TenantID(c), c.Params("id")); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"status": "queued"})
}

func (h *Handler) SetEnabled(c *fiber.Ctx) error {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if err := h.svc.SetEnabled(c.Context(), middleware.TenantID(c), c.Params("id"), body.Enabled); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"enabled": body.Enabled})
}

// UpdateChunk edits a chunk's content and re-embeds it so the new text is
// immediately searchable. The chunkId path param selects the chunk; the body
// carries the new content.
func (h *Handler) UpdateChunk(c *fiber.Ctx) error {
	var body struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	chunk, err := h.svc.UpdateChunkContent(c.Context(), middleware.TenantID(c), c.Params("chunkId"), body.Content)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, chunk)
}

// DeleteChunk removes a single chunk and its Milvus vector.
func (h *Handler) DeleteChunk(c *fiber.Ctx) error {
	if err := h.svc.DeleteChunk(c.Context(), middleware.TenantID(c), c.Params("chunkId")); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}
