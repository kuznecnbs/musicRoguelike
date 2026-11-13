package render

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"soundcheck/internal/core"
)

const sampleRate = 44100

type SoundBank struct {
	ctx     *audio.Context
	samples map[core.SoundType][]byte
}

func NewSoundBank() *SoundBank {
	ctx := audio.NewContext(sampleRate)
	sb := &SoundBank{
		ctx:     ctx,
		samples: make(map[core.SoundType][]byte),
	}
	sb.precomputeSounds()
	return sb
}

func (sb *SoundBank) precomputeSounds() {
	rng := rand.New(rand.NewSource(1959))

	// 1. Bass Wave: deep low-frequency sub-bass drop + warm overtone
	sb.samples[core.SoundBassWave] = synthPCM(0.18, func(t, dur float64) float64 {
		env := math.Pow(1.0-t/dur, 0.7)
		freq := 115.0 - 55.0*(t/dur)
		s := math.Sin(2*math.Pi*freq*t)*0.75 + squareWave(2*math.Pi*freq*0.5*t)*0.25
		return s * env * 0.45
	})

	// 2. Slap: sharp metallic pop + massive sub-punch
	sb.samples[core.SoundSlap] = synthPCM(0.28, func(t, dur float64) float64 {
		env := math.Pow(1.0-t/dur, 1.1)
		freq := 180.0 * math.Exp(-t*10.0)
		noise := (rng.Float64()*2 - 1) * math.Exp(-t*28.0) * 0.5
		sub := math.Sin(2 * math.Pi * (65.0 + freq) * t)
		return (sub*0.8 + noise) * env * 0.55
	})

	// 3. Vocal Hit (Mic sweep + Scream): gritty mid-high chord + breathy noise
	sb.samples[core.SoundVocalHit] = synthPCM(0.16, func(t, dur float64) float64 {
		env := math.Pow(1.0-t/dur, 0.8)
		f1 := 340.0 + math.Sin(t*55.0)*18.0
		f2 := 510.0
		w := (squareWave(2*math.Pi*f1*t)*0.45 + math.Sin(2*math.Pi*f2*t)*0.45 + (rng.Float64()*2-1)*0.15)
		return w * env * 0.35
	})

	// 4. Stage Dive: rising pitch whoosh into heavy thud
	sb.samples[core.SoundStageDive] = synthPCM(0.24, func(t, dur float64) float64 {
		env := 1.0 - t/dur
		freq := 160.0 + 260.0*(t/dur)
		return (math.Sin(2*math.Pi*freq*t)*0.6 + (rng.Float64()*2-1)*0.25) * env * 0.42
	})

	// 5. Guitar Riff: bright overdriven square/sawtooth pluck
	sb.samples[core.SoundGuitarRiff] = synthPCM(0.13, func(t, dur float64) float64 {
		env := math.Exp(-t * 18.0)
		freq := 440.0
		saw := 2.0*(t*freq-math.Floor(0.5+t*freq))
		sq := squareWave(2 * math.Pi * freq * 1.5 * t)
		return (saw*0.6 + sq*0.4) * env * 0.35
	})

	// 6. Drum Hit: punchy snare/tom transient
	sb.samples[core.SoundDrumHit] = synthPCM(0.10, func(t, dur float64) float64 {
		env := math.Exp(-t * 26.0)
		tone := math.Sin(2 * math.Pi * (190.0 * math.Exp(-t*12.0)) * t)
		snare := (rng.Float64()*2 - 1) * 0.45
		return (tone*0.65 + snare) * env * 0.40
	})

	// 7. Cymbal Throw: metallic high-frequency ring shimmer
	sb.samples[core.SoundCymbalThrow] = synthPCM(0.22, func(t, dur float64) float64 {
		env := math.Exp(-t * 10.0)
		m1 := squareWave(2 * math.Pi * 880.0 * t)
		m2 := squareWave(2 * math.Pi * 1318.5 * t)
		noise := (rng.Float64()*2 - 1) * 0.4
		return (m1*0.3 + m2*0.3 + noise) * env * 0.30
	})

	// 8. Wall Slam: heavy low crunch impact
	sb.samples[core.SoundWallSlam] = synthPCM(0.22, func(t, dur float64) float64 {
		env := math.Exp(-t * 11.0)
		sub := math.Sin(2 * math.Pi * 72.0 * t)
		crunch := (rng.Float64()*2 - 1) * math.Exp(-t*18.0)
		return (sub*0.75 + crunch*0.55) * env * 0.55
	})

	// 9. Enemy Death: short descending arcade pop
	sb.samples[core.SoundEnemyDeath] = synthPCM(0.12, func(t, dur float64) float64 {
		env := 1.0 - t/dur
		freq := 300.0 * (1.0 - 0.65*(t/dur))
		return math.Sin(2*math.Pi*freq*t) * env * 0.30
	})

	// 10. Player Hit: dissonant low buzz
	sb.samples[core.SoundPlayerHit] = synthPCM(0.20, func(t, dur float64) float64 {
		env := 1.0 - t/dur
		return (squareWave(2*math.Pi*110.0*t)*0.5 + squareWave(2*math.Pi*117.0*t)*0.5) * env * 0.42
	})

	// 11. Jam Start: triumphant power chord arpeggio
	sb.samples[core.SoundJamStart] = synthPCM(0.42, func(t, dur float64) float64 {
		env := math.Pow(1.0-t/dur, 0.6)
		f := 220.0
		if t > 0.12 {
			f = 330.0
		}
		if t > 0.24 {
			f = 440.0
		}
		return (squareWave(2*math.Pi*f*t)*0.5 + math.Sin(2*math.Pi*f*0.5*t)*0.5) * env * 0.38
	})

	// 12. Upgrade Pick / UI Chime
	sb.samples[core.SoundUpgradePick] = synthPCM(0.18, func(t, dur float64) float64 {
		env := 1.0 - t/dur
		f := 523.25
		if t > 0.08 {
			f = 659.25
		}
		return math.Sin(2*math.Pi*f*t) * env * 0.32
	})

	// 13. Revive Complete
	sb.samples[core.SoundRevive] = synthPCM(0.32, func(t, dur float64) float64 {
		env := 1.0 - t/dur
		f := 330.0 + 330.0*(t/dur)
		return math.Sin(2*math.Pi*f*t) * env * 0.38
	})
}

func (sb *SoundBank) Play(st core.SoundType) {
	if sb == nil || sb.ctx == nil {
		return
	}
	pcm, ok := sb.samples[st]
	if !ok || len(pcm) == 0 {
		return
	}
	p := sb.ctx.NewPlayerFromBytes(pcm)
	p.Play()
}

func squareWave(phase float64) float64 {
	if math.Sin(phase) >= 0 {
		return 1.0
	}
	return -1.0
}

func synthPCM(durationSec float64, gen func(t, dur float64) float64) []byte {
	numSamples := int(durationSec * sampleRate)
	buf := bytes.NewBuffer(make([]byte, 0, numSamples*4))
	for i := 0; i < numSamples; i++ {
		t := float64(i) / sampleRate
		val := gen(t, durationSec)
		if val > 1.0 {
			val = 1.0
		} else if val < -1.0 {
			val = -1.0
		}
		sample := int16(val * 32767)
		// Stereo 16-bit Little Endian PCM
		_ = binary.Write(buf, binary.LittleEndian, sample)
		_ = binary.Write(buf, binary.LittleEndian, sample)
	}
	return buf.Bytes()
}
