package protocol

import "testing"

func TestEnvelopeRoundTrip(t *testing.T) {
	env, err := NewEnvelope(ClientIdentify, IdentifyPayload{Handle: "alice"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}

	payload, err := DecodePayload[IdentifyPayload](env)
	if err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if payload.Handle != "alice" {
		t.Fatalf("expected handle alice, got %q", payload.Handle)
	}
}
