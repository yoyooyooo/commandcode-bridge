package runtime

import "testing"

func TestValidateListenAddr(t *testing.T) {
	if err := ValidateListenAddr("127.0.0.1:8788", false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateListenAddr("localhost:8788", false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateListenAddr("0.0.0.0:8788", false); err == nil {
		t.Fatal("external bind should fail closed")
	}
	if err := ValidateListenAddr("0.0.0.0:8788", true); err != nil {
		t.Fatal(err)
	}
}
