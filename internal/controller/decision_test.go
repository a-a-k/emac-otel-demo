package controller

import "testing"

func TestFullEmaC(t *testing.T) {
	if FullEmaC(false, 1, 1, .95) != Review {
		t.Fatal("inadmissible must review")
	}
	if FullEmaC(true, .96, .99, .95) != Pass {
		t.Fatal("expected pass")
	}
	if FullEmaC(true, .8, .94, .95) != Block {
		t.Fatal("expected block")
	}
}
