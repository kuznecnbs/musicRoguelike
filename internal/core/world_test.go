package core

import "testing"

func TestSimulationFlow(t *testing.T) {
	w := NewWorld()
	if w.Stage != StageLobby {
		t.Fatalf("expected StageLobby, got %v", w.Stage)
	}

	// Select class and ready up solo on Host
	w.SetInput(0, PlayerInput{SelectClass: 1, ReadyToggle: true})
	w.Update()

	if w.Stage != StageCombat {
		t.Fatalf("expected StageCombat after host ready, got %v", w.Stage)
	}

	// Simulate 120 ticks of combat with attacks
	for i := 0; i < 120; i++ {
		w.SetInput(0, PlayerInput{
			MoveX:     1,
			CursorX:   600,
			CursorY:   360,
			Primary:   i%15 == 0,
			Secondary: i == 30,
		})
		w.Update()
		_ = w.DrainSnapshot()
	}

	if len(w.Enemies) == 0 && w.enemiesToSpawn == 0 {
		t.Fatalf("expected enemies to spawn during wave 1")
	}
}
