package handler

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"unicorn/internal/adapter/http/dto"
	"unicorn/internal/domain"
	"unicorn/internal/usecase"
)

type NoteHandler struct {
	uc       usecase.NoteUseCase
	validate *validator.Validate
}

func NewNoteHandler(uc usecase.NoteUseCase, validate *validator.Validate) *NoteHandler {
	return &NoteHandler{
		uc:       uc,
		validate: validate,
	}
}

func (h *NoteHandler) RegisterRoutes(api fiber.Router) {
	notes := api.Group("/notes")
	notes.Post("/", h.Create)
	notes.Get("/", h.List)
	notes.Get("/:id", h.GetByID)
}

func (h *NoteHandler) Create(c fiber.Ctx) error {
	req, err := BindBodyAndValidate[dto.CreateNoteRequest](c, h.validate)
	if err != nil {
		return err
	}

	note, err := h.uc.CreateNote(c.Context(), usecase.CreateNoteInput{
		Title: req.Title,
		Body:  req.Body,
	})
	if err != nil {
		return err
	}

	c.Set(fiber.HeaderLocation, "/api/v1/notes/"+note.ID.String())
	return c.Status(http.StatusCreated).JSON(dto.ToNoteResponse(note))
}

func (h *NoteHandler) GetByID(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return domain.NewValidationError(
			"invalid path parameter",
			domain.FieldError{Field: "id", Tag: "uuid", Message: "id must be a valid UUID"},
		)
	}

	note, err := h.uc.GetNote(c.Context(), id)
	if err != nil {
		return err
	}

	return c.Status(http.StatusOK).JSON(dto.ToNoteResponse(note))
}

func (h *NoteHandler) List(c fiber.Ctx) error {
	q, err := BindQueryAndValidate[dto.ListNotesQuery](c, h.validate)
	if err != nil {
		return err
	}

	notes, err := h.uc.ListNotes(c.Context(), usecase.ListNotesInput{
		Limit:  q.Limit,
		Offset: q.Offset,
	})
	if err != nil {
		return err
	}

	items := make([]dto.NoteResponse, 0, len(notes))
	for _, n := range notes {
		items = append(items, dto.ToNoteResponse(n))
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	return c.Status(http.StatusOK).JSON(dto.PageResponse[dto.NoteResponse]{
		Items:  items,
		Limit:  limit,
		Offset: q.Offset,
	})
}
