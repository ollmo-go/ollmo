package chat

import (
	"bufio"
	"encoding/json"
	"log"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/middleware"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Create(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	if kbID == "" {
		return response.Fail(c, errs.BadRequest("kb_id is required"))
	}
	var in CreateInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	conv, err := h.svc.Create(c.Context(), middleware.TenantID(c), middleware.UserID(c), kbID, in)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.Created(c, conv)
}

func (h *Handler) Get(c *fiber.Ctx) error {
	conv, err := h.svc.Get(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, conv)
}

func (h *Handler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	size, _ := strconv.Atoi(c.Query("size", "20"))
	tenantID := middleware.TenantID(c)
	userID := middleware.UserID(c)
	query := c.Query("q")
	var items []*Conversation
	var total int64
	var err error
	if query != "" {
		items, total, err = h.svc.Search(c.Context(), tenantID, userID, query, page, size)
	} else {
		items, total, err = h.svc.List(c.Context(), tenantID, userID, page, size)
	}
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{
		"items": items, "total": total, "page": page, "size": size,
	})
}

func (h *Handler) Rename(c *fiber.Ctx) error {
	var in struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	conv, err := h.svc.Rename(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"), in.Title)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, conv)
}

func (h *Handler) SetPinned(c *fiber.Ctx) error {
	var in struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	conv, err := h.svc.SetPinned(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"), in.Pinned)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, conv)
}

func (h *Handler) Export(c *fiber.Ctx) error {
	title, md, err := h.svc.Export(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	filename := title + ".md"
	// FormatMediaType quotes/encodes the filename (RFC 2231) so a title
	// containing quotes, CR/LF or non-ASCII characters cannot break or
	// inject extra response headers.
	c.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Set("Content-Type", "text/markdown; charset=utf-8")
	return c.SendString(md)
}

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	msgs, err := h.svc.ListMessages(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"))
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"items": msgs})
}

// VoteMessage records like/dislike feedback on an assistant message. Sending
// the same vote again clears it (vote="").
func (h *Handler) VoteMessage(c *fiber.Ctx) error {
	var in struct {
		Vote string `json:"vote"`
	}
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if err := h.svc.VoteMessage(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"), c.Params("messageId"), in.Vote); err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, fiber.Map{"vote": in.Vote})
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id")); err != nil {
		return response.Fail(c, err)
	}
	return response.NoContent(c)
}

// MessageQuota returns the effective daily message remaining and the user's
// personal limit. remaining=-1 means unlimited.
func (h *Handler) MessageQuota(c *fiber.Ctx) error {
	remaining, userLimit, err := h.svc.MessageUsage(middleware.TenantID(c), middleware.UserID(c))
	if err != nil {
		return response.Fail(c, errs.Wrap(errs.CodeInternal, "get message quota", err))
	}
	return response.OK(c, fiber.Map{
		"remaining": remaining,
		"quota":     userLimit,
	})
}

// Stream sends a user message and streams the assistant reply back as SSE.
//
// Event format (one JSON object per `data:` line):
//
//	data: {"phase":"retrieve","citations":[...]}
//	data: {"phase":"generate","token":"Hel"}
//	data: {"phase":"generate","token":"lo"}
//	data: {"phase":"done","message_id":"..."}
//
// The connection stays open until `done` or `error`. The client cancels by
// closing the connection; ctx cancellation propagates to the LLM stream.
func (h *Handler) Stream(c *fiber.Ctx) error {
	var in SendInput
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	ch, err := h.svc.Stream(c.Context(), middleware.TenantID(c), middleware.UserID(c), c.Params("id"), in)
	if err != nil {
		return response.Fail(c, err)
	}
	writeSSE(c, ch)
	return nil
}

// TestChat streams an agent reply without persisting anything. Used by the
// agent config test drawer so users can try prompts/settings without creating
// throwaway conversations. The response format is identical to Stream, but
// the "done" event has no message_id (nothing was persisted).
func (h *Handler) TestChat(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	if kbID == "" {
		return response.Fail(c, errs.BadRequest("kb_id is required"))
	}
	var in struct {
		Message string `json:"message"`
		Query   string `json:"query"`
	}
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	query := in.Query
	if query == "" {
		query = in.Message
	}
	if query == "" {
		return response.Fail(c, errs.BadRequest("message is required"))
	}
	ch, err := h.svc.TestStream(c.Context(), middleware.TenantID(c), middleware.UserID(c), kbID, query)
	if err != nil {
		return response.Fail(c, err)
	}
	writeSSE(c, ch)
	return nil
}

// Subscribe streams live conversation events to a client viewing the
// conversation: user questions and assistant tokens published by another
// client's Stream call are fanned out here (observer pattern), so multiple
// tabs/devices stay in sync. The connection stays open (heartbeat kept)
// until the client disconnects.
func (h *Handler) Subscribe(c *fiber.Ctx) error {
	convID := c.Params("id")
	ch, err := h.svc.Subscribe(middleware.TenantID(c), middleware.UserID(c), convID)
	if err != nil {
		return response.Fail(c, err)
	}
	writeSSEOnEnd(c, ch, func() { h.svc.hub.Unsubscribe(convID, ch) })
	return nil
}

// DebugNode runs a single agent node in isolation (canvas "test this node").
// Returns the node's output without walking the graph or persisting anything.
func (h *Handler) DebugNode(c *fiber.Ctx) error {
	kbID := c.Params("kbId")
	if kbID == "" {
		return response.Fail(c, errs.BadRequest("kb_id is required"))
	}
	var in struct {
		Node  agent.Node `json:"node"`
		Query string     `json:"query"`
	}
	if err := c.BodyParser(&in); err != nil {
		return response.Fail(c, errs.BadRequest("invalid body: "+err.Error()))
	}
	if in.Node.Type == "" {
		return response.Fail(c, errs.BadRequest("node is required"))
	}
	if strings.TrimSpace(in.Query) == "" {
		return response.Fail(c, errs.BadRequest("query is required"))
	}
	res, err := h.svc.DebugAgentNode(c.Context(), middleware.TenantID(c), kbID, in.Node, in.Query)
	if err != nil {
		return response.Fail(c, err)
	}
	return response.OK(c, res)
}

// sseHeartbeat is the interval between SSE comment heartbeats. A package
// variable so tests can shorten it.
var sseHeartbeat = 30 * time.Second

func writeSSE(c *fiber.Ctx, ch <-chan StreamReply) {
	writeSSEOnEnd(c, ch, nil)
}

// writeSSEOnEnd is writeSSE with an onEnd callback invoked once the stream
// writer stops (client disconnect or channel close). Used by Subscribe to
// release its hub registration; the callback runs inside the body writer
// goroutine, i.e. after the handler has already returned.
func writeSSEOnEnd(c *fiber.Ctx, ch <-chan StreamReply, onEnd func()) {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetContentType("text/event-stream; charset=utf-8")
	c.Context().Response.SetBodyStreamWriter(func(w *bufio.Writer) {
		if onEnd != nil {
			defer onEnd()
		}
		ticker := time.NewTicker(sseHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case reply, ok := <-ch:
				if !ok {
					return
				}
				b, err := json.Marshal(reply)
				if err != nil {
					log.Printf("[chat] sse marshal failed: %v", err)
					continue
				}
				_, _ = w.WriteString("data: ")
				_, _ = w.Write(b)
				_, _ = w.WriteString("\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := w.WriteString(": ping\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
}
