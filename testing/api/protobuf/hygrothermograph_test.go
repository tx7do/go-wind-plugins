package api

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ---------------------------------------------------------------------------
// Field accessors
// ---------------------------------------------------------------------------

func TestHygrothermograph_Accessors(t *testing.T) {
	m := &Hygrothermograph{Humidity: "55.5", Temperature: "23.75"}

	if got := m.GetHumidity(); got != "55.5" {
		t.Errorf("GetHumidity() = %q, want %q", got, "55.5")
	}
	if got := m.GetTemperature(); got != "23.75" {
		t.Errorf("GetTemperature() = %q, want %q", got, "23.75")
	}
}

func TestHygrothermograph_NilReceiverAccessors(t *testing.T) {
	// Generated getters must tolerate a nil receiver.
	var m *Hygrothermograph
	if got := m.GetHumidity(); got != "" {
		t.Errorf("(*Hygrothermograph)(nil).GetHumidity() = %q, want empty", got)
	}
	if got := m.GetTemperature(); got != "" {
		t.Errorf("(*Hygrothermograph)(nil).GetTemperature() = %q, want empty", got)
	}
}

func TestHygrothermograph_Reset(t *testing.T) {
	m := &Hygrothermograph{Humidity: "40", Temperature: "18"}
	m.Reset()

	if m.Humidity != "" || m.Temperature != "" {
		t.Errorf("after Reset = %+v, want zero value", *m)
	}
}

func TestHygrothermograph_ProtoMessage(t *testing.T) {
	var m proto.Message = &Hygrothermograph{}

	if name := m.ProtoReflect().Descriptor().FullName(); name != protoreflect.FullName("protobuf.api.Hygrothermograph") {
		t.Errorf("descriptor full name = %q, want %q", name, "protobuf.api.Hygrothermograph")
	}
}

// ---------------------------------------------------------------------------
// Codec roundtrip
// ---------------------------------------------------------------------------

func TestHygrothermograph_ProtoRoundTrip(t *testing.T) {
	want := &Hygrothermograph{Humidity: "60.5", Temperature: "26.25"}

	data, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("proto.Marshal returned error: %v", err)
	}

	got := &Hygrothermograph{}
	if err := proto.Unmarshal(data, got); err != nil {
		t.Fatalf("proto.Unmarshal returned error: %v", err)
	}
	if got.GetHumidity() != want.GetHumidity() {
		t.Errorf("humidity = %q, want %q", got.GetHumidity(), want.GetHumidity())
	}
	if got.GetTemperature() != want.GetTemperature() {
		t.Errorf("temperature = %q, want %q", got.GetTemperature(), want.GetTemperature())
	}
	if !proto.Equal(want, got) {
		t.Errorf("proto.Equal = false, want true")
	}
}

func TestHygrothermograph_ProtoRoundTrip_Empty(t *testing.T) {
	// An empty message must marshal and unmarshal cleanly.
	data, err := proto.Marshal(&Hygrothermograph{})
	if err != nil {
		t.Fatalf("proto.Marshal returned error: %v", err)
	}

	got := &Hygrothermograph{}
	if err := proto.Unmarshal(data, got); err != nil {
		t.Fatalf("proto.Unmarshal returned error: %v", err)
	}
	if got.Humidity != "" || got.Temperature != "" {
		t.Errorf("decoded = %+v, want zero value", *got)
	}
}

// ---------------------------------------------------------------------------
// gRPC service plumbing
// ---------------------------------------------------------------------------

func TestHygrothermographService_FullMethodName(t *testing.T) {
	want := "/protobuf.api.HygrothermographService/GetHygrothermograph"
	if got := HygrothermographService_GetHygrothermograph_FullMethodName; got != want {
		t.Errorf("full method name = %q, want %q", got, want)
	}
	if got := HygrothermographService_ServiceDesc.ServiceName; got != "protobuf.api.HygrothermographService" {
		t.Errorf("service name = %q, want %q", got, "protobuf.api.HygrothermographService")
	}
}

// unimplementedServer is the forward-compatibility embedding the generated
// code requires.
type unimplementedServer struct {
	UnimplementedHygrothermographServiceServer
}

var _ HygrothermographServiceServer = (*unimplementedServer)(nil)

func TestUnimplementedServer_ReturnsUnimplemented(t *testing.T) {
	srv := &unimplementedServer{}

	_, err := srv.GetHygrothermograph(context.Background(), nil)
	if err == nil {
		t.Fatal("Unimplemented server should return an error")
	}
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("code = %v, want Unimplemented", status.Code(err))
	}
}

func TestRegisterHygrothermographServiceServer(t *testing.T) {
	// Registration must succeed on a plain (not yet serving) gRPC server.
	s := grpc.NewServer()
	defer s.Stop()

	RegisterHygrothermographServiceServer(s, &unimplementedServer{})

	found := false
	for name := range s.GetServiceInfo() {
		if name == "protobuf.api.HygrothermographService" {
			found = true
		}
	}
	if !found {
		t.Error("registered service should appear in GetServiceInfo")
	}
}
