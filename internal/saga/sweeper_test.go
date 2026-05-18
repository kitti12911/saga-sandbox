package saga

import "testing"

func TestDecideSweepAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		attempts int
		max      int
		want     sweepAction
	}{
		{"first attempt redrives", 0, 5, actionRedrive},
		{"below max redrives", 4, 5, actionRedrive},
		{"at max escalates", 5, 5, actionEscalate},
		{"over max escalates", 7, 5, actionEscalate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := decideSweepAction(tc.attempts, tc.max); got != tc.want {
				t.Fatalf("attempts=%d max=%d: got %v want %v",
					tc.attempts, tc.max, got, tc.want)
			}
		})
	}
}
