package apperr

import (
	"errors"
	"fmt"
	"testing"
)

func TestApperrMessage(t *testing.T) {
	cause := errors.New("disk on fire")
	err := fmt.Errorf("sync: %w", New("Etwas ging schief.", cause))
	if Message(err) != "Etwas ging schief." {
		t.Fatal(Message(err))
	}
	if !errors.Is(err, cause) {
		t.Fatal("cause not unwrapped")
	}
	if Message(errors.New("x")) != "Ein unerwarteter Fehler ist aufgetreten." {
		t.Fatal("default message")
	}
}
