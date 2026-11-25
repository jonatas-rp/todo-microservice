package todo

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestServiceCRUD(t *testing.T) {
	t.Parallel()
	svc := New(newMemRepo())
	ctx := context.Background()

	const key = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	created, createdNew, err := svc.Create(ctx, "  buy milk  ", false, key)
	if !createdNew {
		t.Fatal("Create created = false")
	}
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	parsed, err := uuid.Parse(created.PublicID)
	if err != nil || parsed.Version() != 4 || created.ID == 0 {
		t.Fatalf("created = %+v", created)
	}
	if created.Title != "buy milk" || created.Completed {
		t.Fatalf("created = %+v", created)
	}
	replay, replayNew, err := svc.Create(ctx, "other title", true, key)
	if err != nil || replayNew || replay != created {
		t.Fatalf("replay = %+v %v %v", replay, replayNew, err)
	}

	items, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0] != created {
		t.Fatalf("List = %+v", items)
	}

	updated, err := svc.Update(ctx, created.PublicID, "buy oats", true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.PublicID != created.PublicID || updated.Title != "buy oats" || !updated.Completed {
		t.Fatalf("updated = %+v", updated)
	}

	if err := svc.Delete(ctx, created.PublicID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	items, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List after delete = %+v", items)
	}
}

func TestServiceValidation(t *testing.T) {
	t.Parallel()
	svc := New(newMemRepo())
	ctx := context.Background()

	if _, _, err := svc.Create(ctx, "   ", false, "6ba7b810-9dad-11d1-80b4-00c04fd430c8"); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("Create blank title: %v", err)
	}
	if _, _, err := svc.Create(ctx, "title", false, "  "); !errors.Is(err, ErrIdempotencyKey) {
		t.Fatalf("Create blank key: %v", err)
	}
	if _, _, err := svc.Create(ctx, "title", false, "not-a-uuid"); !errors.Is(err, ErrIdempotencyKey) {
		t.Fatalf("Create invalid key: %v", err)
	}
	if _, err := svc.Update(ctx, "missing", "title", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update missing: %v", err)
	}
	if err := svc.Delete(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete missing: %v", err)
	}
	if _, err := svc.Update(ctx, "id", "  ", true); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("Update blank title: %v", err)
	}
	if _, err := svc.Update(ctx, "", "title", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update empty id: %v", err)
	}
}

func TestNewTodo(t *testing.T) {
	t.Parallel()
	a, err := newTodo("  buy milk ", true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newTodo("buy oats", false)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(a.PublicID)
	if err != nil || parsed.Version() != 4 || a.ID != 0 || a.PublicID == b.PublicID {
		t.Fatalf("newTodo = %+v, %+v", a, b)
	}
	if a.Title != "buy milk" || !a.Completed {
		t.Fatalf("newTodo = %+v", a)
	}
	if _, err := newTodo(" ", false); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("newTodo blank: %v", err)
	}
}

func TestListNilRepoResult(t *testing.T) {
	t.Parallel()
	repo := newMemRepo()
	repo.nilList = true
	items, err := New(repo).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("List = %#v", items)
	}
}

func TestServiceCanceledContext(t *testing.T) {
	t.Parallel()
	svc := New(newMemRepo())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List: %v", err)
	}
	if _, _, err := svc.Create(ctx, "title", false, "6ba7b810-9dad-11d1-80b4-00c04fd430c8"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Update(ctx, "id", "title", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("Update: %v", err)
	}
	if err := svc.Delete(ctx, "id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete: %v", err)
	}
}

func TestServiceIdempotencyKeyCanonical(t *testing.T) {
	t.Parallel()
	svc := New(newMemRepo())
	ctx := context.Background()

	created, createdNew, err := svc.Create(ctx, "buy milk", false, "6BA7B810-9DAD-11D1-80B4-00C04FD430C8")
	if err != nil || !createdNew {
		t.Fatalf("Create = %+v %v %v", created, createdNew, err)
	}
	replay, replayNew, err := svc.Create(ctx, "other", true, "6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	if err != nil || replayNew || replay != created {
		t.Fatalf("replay = %+v %v %v", replay, replayNew, err)
	}
}

type memRepo struct {
	items   map[string]Todo
	keys    map[string]string
	order   []string
	next    uint64
	nilList bool
}

func newMemRepo() *memRepo {
	return &memRepo{items: map[string]Todo{}, keys: map[string]string{}}
}

func (m *memRepo) List(context.Context) ([]Todo, error) {
	if m.nilList {
		return nil, nil
	}
	out := make([]Todo, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.items[id])
	}
	return out, nil
}

func (m *memRepo) Create(_ context.Context, item Todo, idempotencyKey string) (Todo, bool, error) {
	if idempotencyKey == "" {
		return Todo{}, false, ErrIdempotencyKey
	}
	if id, ok := m.keys[idempotencyKey]; ok {
		return m.items[id], false, nil
	}
	if _, ok := m.items[item.PublicID]; ok {
		return Todo{}, false, errors.New("already exists")
	}
	if item.ID == 0 {
		m.next++
		item.ID = m.next
	}
	m.items[item.PublicID] = item
	m.keys[idempotencyKey] = item.PublicID
	m.order = append(m.order, item.PublicID)
	return item, true, nil
}

func (m *memRepo) Update(_ context.Context, item Todo) error {
	current, ok := m.items[item.PublicID]
	if !ok {
		return ErrNotFound
	}
	current.Title = item.Title
	current.Completed = item.Completed
	m.items[item.PublicID] = current
	return nil
}

func (m *memRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.items[id]; !ok {
		return ErrNotFound
	}
	delete(m.items, id)
	for i, existing := range m.order {
		if existing == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	return nil
}
