package worker

import (
	"context"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"unicorn/internal/usecase"
)

type NoteCreatedArgs struct {
	NoteID uuid.UUID `json:"note_id"`
}

func (NoteCreatedArgs) Kind() string {
	return "note_created"
}

type NoteCreatedWorker struct {
	river.WorkerDefaults[NoteCreatedArgs]
	processor usecase.NoteJobProcessor
}

func NewNoteCreatedWorker(processor usecase.NoteJobProcessor) *NoteCreatedWorker {
	return &NoteCreatedWorker{processor: processor}
}

func (w *NoteCreatedWorker) Work(ctx context.Context, job *river.Job[NoteCreatedArgs]) error {
	return w.processor.ProcessNoteCreated(ctx, job.Args.NoteID)
}

func RegisterWorkers(processor usecase.NoteJobProcessor) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewNoteCreatedWorker(processor))
	return workers
}
