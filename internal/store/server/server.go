// Package server adapts the BoltDB store to the gRPC Store service.
package server

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"todo-service/internal/logger"
	"todo-service/internal/storage"
	"todo-service/internal/store/pb"
	"todo-service/internal/todo"
)

// Server serves Repository operations over gRPC.
type Server struct {
	pb.UnimplementedStoreServer
	store *storage.Store
	log   *logger.Logger
}

// New returns a server that reads and writes store.
func New(store *storage.Store, log *logger.Logger) *Server {
	return &Server{store: store, log: log}
}

// List returns every stored to-do.
func (s *Server) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	log := s.requestLogger(req.GetRequestId())
	items, err := s.store.List(ctx)
	if err != nil {
		log.Error("list", map[string]any{"err": err.Error()})
		return nil, statusErr(err)
	}
	log.Info("list", map[string]any{"count": len(items)})
	out := make([]*pb.Todo, 0, len(items))
	for _, item := range items {
		out = append(out, toPB(item))
	}
	return &pb.ListResponse{Items: out}, nil
}

// Create stores item unless its idempotency key was already used.
func (s *Server) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	log := s.requestLogger(req.GetRequestId())
	if req.GetItem() == nil {
		log.Error("create", map[string]any{"err": "item is required"})
		return nil, status.Error(codes.Internal, "item is required")
	}
	stored, created, err := s.store.Create(ctx, fromPB(req.GetItem()), req.GetIdempotencyKey())
	if err != nil {
		log.Error("create", map[string]any{"err": err.Error()})
		return nil, statusErr(err)
	}
	log.Info("create", map[string]any{"id": stored.PublicID, "created": created})
	return &pb.CreateResponse{Stored: toPB(stored), Created: created}, nil
}

// Update replaces the title and completed flag of an existing to-do.
func (s *Server) Update(ctx context.Context, req *pb.UpdateRequest) (*pb.UpdateResponse, error) {
	log := s.requestLogger(req.GetRequestId())
	if req.GetItem() == nil {
		log.Error("update", map[string]any{"err": "item is required"})
		return nil, status.Error(codes.Internal, "item is required")
	}
	if err := s.store.Update(ctx, fromPB(req.GetItem())); err != nil {
		log.Error("update", map[string]any{"err": err.Error()})
		return nil, statusErr(err)
	}
	log.Info("update", map[string]any{"id": req.GetItem().GetPublicId()})
	return &pb.UpdateResponse{}, nil
}

// Delete removes a to-do by its public id.
func (s *Server) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	log := s.requestLogger(req.GetRequestId())
	if err := s.store.Delete(ctx, req.GetId()); err != nil {
		log.Error("delete", map[string]any{"err": err.Error()})
		return nil, statusErr(err)
	}
	log.Info("delete", map[string]any{"id": req.GetId()})
	return &pb.DeleteResponse{}, nil
}

func (s *Server) requestLogger(requestID string) *logger.Logger {
	return s.log.WithRequestID(requestID)
}

// Ping reports whether the database can serve traffic.
func (s *Server) Ping(ctx context.Context, _ *pb.PingRequest) (*pb.PingResponse, error) {
	if err := s.store.Ping(ctx); err != nil {
		return nil, statusErr(err)
	}
	return &pb.PingResponse{}, nil
}

func statusErr(err error) error {
	if errors.Is(err, todo.ErrNotFound) {
		return status.Error(codes.NotFound, todo.ErrNotFound.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

func toPB(item todo.Todo) *pb.Todo {
	return &pb.Todo{
		Id:        item.ID,
		PublicId:  item.PublicID,
		Title:     item.Title,
		Completed: item.Completed,
	}
}

func fromPB(item *pb.Todo) todo.Todo {
	return todo.Todo{
		ID:        item.GetId(),
		PublicID:  item.GetPublicId(),
		Title:     item.GetTitle(),
		Completed: item.GetCompleted(),
	}
}
