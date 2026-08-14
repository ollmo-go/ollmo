package backup

import (
	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Export(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	data, name, err := h.svc.ExportKB(c.Context(), middleware.TenantID(c), kbID)
	if err != nil {
		return response.Fail(c, err)
	}
	filename := name + ".json"
	c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Set("Content-Type", "application/json")
	return c.Send(data)
}

func (h *Handler) Import(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	file, err := c.FormFile("file")
	if err != nil {
		return response.Fail(c, errs.BadRequest("file is required"))
	}
	if file.Size > 100*1024*1024 {
		return response.Fail(c, errs.BadRequest("backup file too large (max 100MB)"))
	}
	src, err := file.Open()
	if err != nil {
		return response.Fail(c, errs.BadRequest("cannot read file"))
	}
	defer src.Close()
	buf := make([]byte, file.Size)
	if _, err := src.Read(buf); err != nil {
		return response.Fail(c, errs.BadRequest("cannot read file content"))
	}
	n, err := h.svc.ImportKB(c.Context(), middleware.TenantID(c), kbID, buf)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"imported_documents": n})
}
