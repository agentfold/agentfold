package store

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toProject(r sqlcgen.Project) domain.Project {
	return domain.Project{
		ID:         r.ID,
		Name:       r.Name,
		SourceKind: domain.SourceKind(r.SourceKind),
		LocalPath:  r.LocalPath,
		CreatedAt:  r.CreatedAt,
	}
}

// ListProjects returns the projects ordered by name.
func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.q.ListProjects(ctx)
	return mapRows(rows, err, toProject)
}

// GetProject returns one project.
func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	row, err := s.q.GetProject(ctx, id)
	return mapRow(row, err, toProject)
}

// CreateProject stores a local project.
func (s *Store) CreateProject(ctx context.Context, in domain.ProjectInput) (domain.Project, error) {
	row, err := s.q.InsertProject(ctx, sqlcgen.InsertProjectParams{
		Name:       in.Name,
		SourceKind: string(domain.SourceLocal),
		LocalPath:  in.LocalPath,
	})
	return mapRow(row, err, toProject)
}

// UpdateProject changes the name and path of a project.
func (s *Store) UpdateProject(ctx context.Context, id uuid.UUID, in domain.ProjectInput) (domain.Project, error) {
	row, err := s.q.UpdateProject(ctx, sqlcgen.UpdateProjectParams{ID: id, Name: in.Name, LocalPath: in.LocalPath})
	return mapRow(row, err, toProject)
}

// DeleteProject removes a project no card refers to.
func (s *Store) DeleteProject(ctx context.Context, id uuid.UUID) error {
	return requireDeleted(s.q.DeleteProject(ctx, id))
}
