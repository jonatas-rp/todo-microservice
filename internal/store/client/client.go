// Package client calls the store gRPC service and implements todo.Repository.
package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"todo-service/internal/store/pb"
	"todo-service/internal/todo"
)

// Client is a todo.Repository backed by the store gRPC service.
// Ping reports whether that service can serve traffic.
type Client struct {
	conn *grpc.ClientConn
	api  pb.StoreClient
}

// Dial connects to the store at addr. The connection is lazy.
// The caller must Close the client.
func Dial(ctx context.Context, addr string) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial store: %w", err)
	}
	return &Client{conn: conn, api: pb.NewStoreClient(conn)}, nil
}

// Close releases the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// List returns every to-do. The gRPC request carries the id from ctx.
func (c *Client) List(ctx context.Context) ([]todo.Todo, error) {
	requestID, _ := ctx.Value("request_id").(string)
	resp, err := c.api.List(ctx, &pb.ListRequest{RequestId: requestID})
	if err != nil {
		return nil, fromStatus(err)
	}
	items := make([]todo.Todo, 0, len(resp.GetItems()))
	for _, item := range resp.GetItems() {
		items = append(items, fromPB(item))
	}
	return items, nil
}

// Create stores item under idempotencyKey.
func (c *Client) Create(ctx context.Context, item todo.Todo, idempotencyKey string) (todo.Todo, bool, error) {
	requestID, _ := ctx.Value("request_id").(string)
	resp, err := c.api.Create(ctx, &pb.CreateRequest{
		Item:           toPB(item),
		IdempotencyKey: idempotencyKey,
		RequestId:      requestID,
	})
	if err != nil {
		return todo.Todo{}, false, fromStatus(err)
	}
	return fromPB(resp.GetStored()), resp.GetCreated(), nil
}

// Update replaces an existing to-do.
func (c *Client) Update(ctx context.Context, item todo.Todo) error {
	requestID, _ := ctx.Value("request_id").(string)
	_, err := c.api.Update(ctx, &pb.UpdateRequest{Item: toPB(item), RequestId: requestID})
	return fromStatus(err)
}

// Delete removes a to-do by its public id.
func (c *Client) Delete(ctx context.Context, id string) error {
	requestID, _ := ctx.Value("request_id").(string)
	_, err := c.api.Delete(ctx, &pb.DeleteRequest{Id: id, RequestId: requestID})
	return fromStatus(err)
}

// Ping checks the store process.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Ping(ctx, &pb.PingRequest{})
	return fromStatus(err)
}

func fromStatus(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return todo.ErrNotFound
	}
	return err
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
	if item == nil {
		return todo.Todo{}
	}
	return todo.Todo{
		ID:        item.GetId(),
		PublicID:  item.GetPublicId(),
		Title:     item.GetTitle(),
		Completed: item.GetCompleted(),
	}
}

var _ todo.Repository = (*Client)(nil)
