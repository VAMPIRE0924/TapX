package pathmtu

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestProbeRequestResponseRoundTrip(t *testing.T) {
	request, err := NewProbeRequest(1400)
	if err != nil {
		t.Fatal(err)
	}
	parsedRequest, err := ParseProbe(request)
	if err != nil {
		t.Fatal(err)
	}
	if parsedRequest.Type != ProbeRequest || parsedRequest.Size != 1400 {
		t.Fatalf("request = %+v", parsedRequest)
	}
	response, err := ProbeResponseFor(request)
	if err != nil {
		t.Fatal(err)
	}
	parsedResponse, err := ParseProbe(response)
	if err != nil {
		t.Fatal(err)
	}
	if parsedResponse.Type != ProbeResponse || parsedResponse.Token != parsedRequest.Token || !bytes.Equal(parsedResponse.Body, parsedRequest.Body) {
		t.Fatalf("response = %+v, request = %+v", parsedResponse, parsedRequest)
	}
	commit, err := ProbeCommitFor(request)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := ProbeCommittedFor(commit)
	if err != nil {
		t.Fatal(err)
	}
	parsedCommitted, err := ParseProbe(committed)
	if err != nil {
		t.Fatal(err)
	}
	if parsedCommitted.Type != ProbeCommitted || parsedCommitted.Token != parsedRequest.Token ||
		!bytes.Equal(parsedCommitted.Body, parsedRequest.Body) {
		t.Fatalf("committed = %+v, request = %+v", parsedCommitted, parsedRequest)
	}
}

func TestParseProbeRejectsTruncationAndCorruption(t *testing.T) {
	request, err := NewProbeRequest(128)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProbe(request[:127]); err == nil {
		t.Fatal("truncated probe was accepted")
	}
	request[80] ^= 0xff
	if _, err := ParseProbe(request); err == nil {
		t.Fatal("corrupt probe was accepted")
	}
}

func TestConfirmPayloadAcceptsDesiredCeiling(t *testing.T) {
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1500, CandidatePayload: 1472, MinimumPayload: 600, Attempts: 1,
	}, echoProbe)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != 1500 || result.ProbeCount != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestConfirmPayloadFallsBackAndBinarySearches(t *testing.T) {
	const pathLimit = 1320
	exchange := func(_ context.Context, request []byte) ([]byte, error) {
		if len(request) > pathLimit {
			return nil, errors.New("probe lost")
		}
		return ProbeResponseFor(request)
	}
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1500, CandidatePayload: 1472, MinimumPayload: 600, Attempts: 1,
	}, exchange)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != pathLimit {
		t.Fatalf("confirmed payload = %d, want %d", result.PayloadSize, pathLimit)
	}
}

func TestConfirmPayloadRetriesTransientLoss(t *testing.T) {
	calls := 0
	exchange := func(_ context.Context, request []byte) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary loss")
		}
		return ProbeResponseFor(request)
	}
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1400, CandidatePayload: 1372, MinimumPayload: 600, Attempts: 2,
	}, exchange)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != 1400 || result.ProbeCount != 2 {
		t.Fatalf("result = %+v, calls = %d", result, calls)
	}
}

func TestConfirmPayloadFailsWithoutKnownGoodSize(t *testing.T) {
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1400, CandidatePayload: 1300, MinimumPayload: 600, Attempts: 1,
	}, func(context.Context, []byte) ([]byte, error) { return nil, errors.New("timeout") })
	if err == nil {
		t.Fatal("ConfirmPayload() error = nil")
	}
	if result.PayloadSize != 0 || result.ProbeCount != 3 {
		t.Fatalf("result = %+v", result)
	}
}

func TestConfirmPayloadRejectsUnrelatedResponse(t *testing.T) {
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 800, CandidatePayload: 700, MinimumPayload: 600, Attempts: 1,
	}, func(context.Context, []byte) ([]byte, error) {
		other, err := NewProbeRequest(800)
		if err != nil {
			return nil, err
		}
		return ProbeResponseFor(other)
	})
	if err == nil {
		t.Fatal("unrelated probe response was accepted")
	}
	if result.PayloadSize != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func echoProbe(_ context.Context, request []byte) ([]byte, error) {
	return ProbeResponseFor(request)
}
