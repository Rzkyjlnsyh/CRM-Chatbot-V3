#!/usr/bin/env node
/**
 * Launcher development lintas platform (macOS, Linux, Windows).
 *
 * Menjalankan:
 *   - Frontend Vite (hot reload) di http://127.0.0.1:5173
 *   - Backend Go via Air (hot reload) di http://127.0.0.1:3030
 *     fallback: go run ./backend jika Air belum terpasang
 *
 * Pemakaian (dari root project):
 *   npm run dev
 *   node scripts/dev.mjs
 *   ./scripts/dev.sh          (macOS/Linux)
 *   scripts\dev.bat           (Windows)
 *   powershell -File scripts/dev.ps1
 */

import { spawn, execSync } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(__dirname, '..');
const IS_WIN = process.platform === 'win32';
const FRONTEND_PORT = 5173;
const BACKEND_PORT = 3030;

const children = [];
let shuttingDown = false;

function log(msg) {
  console.log(msg);
}

function fail(msg, code = 1) {
  console.error(`\n✗ ${msg}\n`);
  process.exit(code);
}

function commandExists(cmd) {
  try {
    if (IS_WIN) {
      execSync(`where ${cmd}`, { stdio: 'ignore', windowsHide: true });
    } else {
      execSync(`command -v ${cmd}`, { stdio: 'ignore', shell: '/bin/bash' });
    }
    return true;
  } catch {
    return false;
  }
}

function resolveBin(name) {
  const candidates = [];
  if (IS_WIN) {
    candidates.push(
      path.join(os.homedir(), 'go', 'bin', `${name}.exe`),
      path.join(os.homedir(), 'go', 'bin', name),
    );
  } else {
    candidates.push(
      path.join(os.homedir(), 'go', 'bin', name),
      path.join('/usr/local/go/bin', name),
    );
  }
  for (const p of candidates) {
    if (fs.existsSync(p)) return p;
  }
  if (commandExists(name)) return name;
  if (IS_WIN && commandExists(`${name}.exe`)) return `${name}.exe`;
  return null;
}

function portFree(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.unref();
    server.once('error', () => resolve(false));
    server.once('listening', () => {
      server.close(() => resolve(true));
    });
    server.listen(port, '127.0.0.1');
  });
}

async function assertPortsFree() {
  for (const port of [BACKEND_PORT, FRONTEND_PORT]) {
    const free = await portFree(port);
    if (!free) {
      fail(
        `Port ${port} masih dipakai proses lain.\n` +
          `  Hentikan server development lama, lalu jalankan lagi.\n` +
          (IS_WIN
            ? `  Tips Windows: netstat -ano | findstr :${port}`
            : `  Tips macOS/Linux: lsof -nP -iTCP:${port} -sTCP:LISTEN`),
      );
    }
  }
}

function ensurePrereqs() {
  if (!commandExists('go') && !commandExists('go.exe')) {
    fail('Go belum terpasang atau tidak ada di PATH. Install dari https://go.dev/dl/');
  }
  if (!commandExists('npm') && !commandExists('npm.cmd')) {
    fail('Node.js/npm belum terpasang atau tidak ada di PATH. Install dari https://nodejs.org/');
  }
  const nm = path.join(ROOT, 'frontend', 'node_modules');
  if (!fs.existsSync(nm)) {
    fail(
      'Dependency frontend belum terpasang.\n' +
        '  Jalankan dulu: npm --prefix frontend install\n' +
        '  atau: npm run setup',
    );
  }
  const envFile = path.join(ROOT, '.env');
  if (!fs.existsSync(envFile)) {
    log('! File .env belum ada. Salin dari .env.example lalu isi kredensial:');
    log(IS_WIN ? '    copy .env.example .env' : '    cp .env.example .env');
  }
}

function ensureAir() {
  let air = resolveBin('air');
  if (air) return air;

  log('Air (hot-reload backend) belum ada. Menginstal otomatis…');
  try {
    execSync('go install github.com/air-verse/air@latest', {
      stdio: 'inherit',
      cwd: ROOT,
      env: process.env,
      windowsHide: true,
    });
  } catch {
    log('! Gagal install Air. Backend akan memakai: go run ./backend (tanpa auto-rebuild).');
    return null;
  }

  air = resolveBin('air');
  if (!air) {
    log('! Air terinstal tapi belum ketemu di PATH. Backend memakai go run ./backend.');
    log(`  Pastikan folder go/bin ada di PATH: ${path.join(os.homedir(), 'go', 'bin')}`);
    return null;
  }
  return air;
}

function spawnProc(command, args, { name, env = {} } = {}) {
  const child = spawn(command, args, {
    cwd: ROOT,
    env: { ...process.env, ...env },
    stdio: 'inherit',
    shell: IS_WIN,
    windowsHide: true,
  });
  child.__name = name || command;
  children.push(child);
  child.on('exit', (code, signal) => {
    if (shuttingDown) return;
    // Frontend/backend mati tak terduga → hentikan yang lain.
    if (signal !== 'SIGTERM' && signal !== 'SIGINT') {
      console.error(`\n✗ Proses ${child.__name} berhenti (code=${code ?? 'null'}, signal=${signal ?? 'null'}).`);
      shutdown(code && code !== 0 ? code : 1);
    }
  });
  return child;
}

function shutdown(code = 0) {
  if (shuttingDown) return;
  shuttingDown = true;
  log('\nMenghentikan server development…');
  for (const child of children) {
    if (!child.killed && child.exitCode === null) {
      try {
        if (IS_WIN) {
          // Kill process tree di Windows.
          spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], {
            stdio: 'ignore',
            windowsHide: true,
          });
        } else {
          child.kill('SIGTERM');
        }
      } catch {
        /* ignore */
      }
    }
  }
  setTimeout(() => process.exit(code), IS_WIN ? 800 : 300);
}

function airConfigPath() {
  return path.join(ROOT, IS_WIN ? '.air.windows.toml' : '.air.toml');
}

async function main() {
  process.chdir(ROOT);
  log('══════════════════════════════════════════════');
  log('  SlaluDiskon | SlaluDiskon — development (lintas OS)');
  log(`  Platform: ${process.platform} / ${os.arch()}`);
  log('══════════════════════════════════════════════\n');

  ensurePrereqs();
  await assertPortsFree();

  const goCache = path.join(ROOT, '.tmp', 'go-cache');
  fs.mkdirSync(goCache, { recursive: true });
  fs.mkdirSync(path.join(ROOT, '.tmp', 'air'), { recursive: true });

  const npmCmd = IS_WIN ? 'npm.cmd' : 'npm';
  log(`→ Frontend Vite  http://127.0.0.1:${FRONTEND_PORT}`);
  spawnProc(npmCmd, ['--prefix', 'frontend', 'run', 'dev', '--', '--host', '127.0.0.1', '--port', String(FRONTEND_PORT), '--strictPort'], {
    name: 'frontend',
  });

  // Beri waktu vite start singkat.
  await new Promise((r) => setTimeout(r, 800));

  const air = ensureAir();
  const childEnv = {
    GOCACHE: goCache,
  };

  if (air) {
    const cfg = airConfigPath();
    if (!fs.existsSync(cfg)) {
      fail(`File konfigurasi Air tidak ditemukan: ${path.relative(ROOT, cfg)}`);
    }
    log(`→ Backend Air   http://127.0.0.1:${BACKEND_PORT}  (config: ${path.basename(cfg)})`);
    log('  Perubahan file Go akan di-build ulang otomatis.\n');
    spawnProc(air, ['-c', cfg], { name: 'backend-air', env: childEnv });
  } else {
    log(`→ Backend       http://127.0.0.1:${BACKEND_PORT}  (go run)`);
    log('  Tip: pasang Air untuk auto-reload: go install github.com/air-verse/air@latest\n');
    const goCmd = IS_WIN ? 'go.exe' : 'go';
    spawnProc(goCmd, ['run', '-ldflags', '-X wa-assistant/backend/license.DevMode=true', './backend'], { name: 'backend-go', env: childEnv });
  }

  log('Tekan Ctrl+C untuk berhenti.\n');
}

for (const sig of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
  try {
    process.on(sig, () => shutdown(0));
  } catch {
    /* SIGHUP tidak selalu ada di Windows */
  }
}

// Windows Ctrl+C
if (IS_WIN) {
  process.on('message', () => {});
}

main().catch((err) => {
  console.error(err);
  shutdown(1);
});
