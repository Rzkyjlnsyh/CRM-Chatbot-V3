// inboxSound.ts — bunyi notifikasi pesan masuk (WAV inline, tanpa aset eksternal).
// Audio hanya bisa diputar setelah interaksi user → unlock saat login.

let audioCtx: AudioContext | null = null;
let unlocked = false;

export function unlockInboxSound() {
  try {
    if (!audioCtx) {
      const Ctor = window.AudioContext || (window as any).webkitAudioContext;
      if (!Ctor) return;
      audioCtx = new Ctor();
    }
    if (audioCtx.state === 'suspended') void audioCtx.resume();
    unlocked = true;
  } catch {
    // Browser tanpa WebAudio — abaikan (fitur kosmetik).
  }
}

export function isInboxSoundUnlocked() {
  return unlocked;
}

// playInboxSound memainkan dua nada pendek lembut (E5→A5), volume rendah.
export function playInboxSound() {
  try {
    if (!audioCtx || audioCtx.state !== 'running') return;
    const now = audioCtx.currentTime;
    const note = (freq: number, start: number, dur: number) => {
      const osc = audioCtx!.createOscillator();
      const gain = audioCtx!.createGain();
      osc.type = 'sine';
      osc.frequency.value = freq;
      gain.gain.setValueAtTime(0.0001, now + start);
      gain.gain.exponentialRampToValueAtTime(0.12, now + start + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, now + start + dur);
      osc.connect(gain).connect(audioCtx!.destination);
      osc.start(now + start);
      osc.stop(now + start + dur + 0.05);
    };
    note(659.25, 0, 0.18); // E5
    note(880.0, 0.16, 0.22); // A5
  } catch {
    // jangan ganggu UI karena audio
  }
}
