package handlers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb_misc "github.com/0xef53/kvmrun/api/services/misc/v2"

	"github.com/0xef53/kvmrun-dashboard/internal/daemon"
)

// fakeMisc implements the MiscService for handler tests: it stores comments
// in memory and returns a permission error for the machine "locked" to
// exercise the error path.
type fakeMisc struct {
	pb_misc.UnimplementedMiscServiceServer
	comments map[string]string
}

func (f *fakeMisc) GetMachineComment(ctx context.Context, req *pb_misc.GetMachineCommentRequest) (*pb_misc.GetMachineCommentResponse, error) {
	c, ok := f.comments[req.Name]
	if !ok {
		return nil, status.Error(codes.NotFound, "comment not found")
	}
	return &pb_misc.GetMachineCommentResponse{Data: []byte(c)}, nil
}

func (f *fakeMisc) UpdateMachineComment(ctx context.Context, req *pb_misc.UpdateMachineCommentRequest) (*emptypb.Empty, error) {
	if req.Name == "locked" {
		return nil, status.Error(codes.PermissionDenied, "machine is in locked state")
	}
	f.comments[req.Name] = string(req.Data)
	return &emptypb.Empty{}, nil
}

// newCommentTestHandlers spins up an in-process gRPC server with the fake
// MiscService and returns handlers wired to it.
func newCommentTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb_misc.RegisterMiscServiceServer(srv, &fakeMisc{comments: map[string]string{}})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	client, err := daemon.New(lis.Addr().String(), nil)
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &Handlers{Daemon: client}
}

func TestGetMachineCommentJSON(t *testing.T) {
	t.Run("absent comment returns empty", func(t *testing.T) {
		h := newCommentTestHandlers(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/machines/web-01/comment", nil)
		c.Params = gin.Params{{Key: "name", Value: "web-01"}}
		h.MachineCommentJSON(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); body != `{"comment":""}` {
			t.Errorf("body = %s, want empty comment", body)
		}
	})

	t.Run("stored comment is returned", func(t *testing.T) {
		h := newCommentTestHandlers(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/machines/web-01/comment", nil)
		c.Params = gin.Params{{Key: "name", Value: "web-01"}}
		// Seed via the daemon directly.
		if _, err := h.Daemon.Misc.UpdateMachineComment(c.Request.Context(),
			&pb_misc.UpdateMachineCommentRequest{Name: "web-01", Data: []byte("test comment")}); err != nil {
			t.Fatalf("seed comment: %v", err)
		}
		h.MachineCommentJSON(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if body := w.Body.String(); !strings.Contains(body, `"comment":"test comment"`) {
			t.Errorf("body = %s, want stored comment", body)
		}
	})
}

func TestUpdateMachineCommentJSON(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h := newCommentTestHandlers(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/machines/web-01/comment", strings.NewReader(`{"comment":"hello"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "name", Value: "web-01"}}
		h.UpdateMachineCommentJSON(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
		}
	})

	t.Run("daemon error is passed through", func(t *testing.T) {
		h := newCommentTestHandlers(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/machines/locked/comment", strings.NewReader(`{"comment":"x"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "name", Value: "locked"}}
		h.UpdateMachineCommentJSON(c)

		if w.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
		}
		body := w.Body.String()
		if !strings.Contains(body, "machine is in locked state") {
			t.Errorf("body = %s, want the kvmrun error text", body)
		}
	})

	t.Run("invalid JSON is rejected", func(t *testing.T) {
		h := newCommentTestHandlers(t)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/machines/web-01/comment", strings.NewReader("not-json"))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "name", Value: "web-01"}}
		h.UpdateMachineCommentJSON(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}
