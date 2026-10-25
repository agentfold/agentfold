package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestProjectCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, domain.ProjectInput{Name: "web", LocalPath: "/src/web"})
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceKind != domain.SourceLocal || p.LocalPath != "/src/web" {
		t.Fatalf("unexpected project %+v", p)
	}
	if _, err := s.CreateProject(ctx, domain.ProjectInput{Name: "api", LocalPath: "/src/api"}); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListProjects(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "api" {
		t.Fatalf("ListProjects = %+v, %v", list, err)
	}
	got, err := s.GetProject(ctx, p.ID)
	if err != nil || got.Name != "web" {
		t.Fatalf("GetProject = %+v, %v", got, err)
	}
	updated, err := s.UpdateProject(ctx, p.ID, domain.ProjectInput{Name: "site", LocalPath: "/src/site"})
	if err != nil || updated.Name != "site" || updated.LocalPath != "/src/site" {
		t.Fatalf("UpdateProject = %+v, %v", updated, err)
	}
	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(ctx, p.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetProject after delete: %v", err)
	}
}

func TestProjectErrors(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in := domain.ProjectInput{Name: "web", LocalPath: "/src/web"}
	if _, err := s.CreateProject(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject(ctx, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if err := s.DeleteProject(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := s.ListProjects(cancelled()); err == nil {
		t.Fatal("ListProjects: expected error")
	}

	f := newFixture(t, s)
	if err := s.DeleteProject(ctx, f.project.ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("DeleteProject in use: %v", err)
	}
}
