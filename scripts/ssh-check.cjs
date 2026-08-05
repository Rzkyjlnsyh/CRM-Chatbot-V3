const { spawn } = require('child_process');

const PASS = process.argv[2] || 'TSdn5t1qoPG0fSh99r26';
const HOST = 'root@103.74.5.62';
const CMD = `
echo "=== HOSTNAME ===" && hostname
echo "=== UPTIME ===" && uptime
echo "=== DISK ===" && df -h /
echo "=== MEMORY ===" && free -h
echo "=== WHOAMI ===" && whoami
echo "=== HOME FILES ===" && ls -la /root/
echo "=== GO BINARY ===" && find /root -name "wa-assistant*" -o -name "*.go" 2>/dev/null | head -10
echo "=== RUNNING PROCS ===" && ps aux | grep -E "wa-assistant|go|mysql|mariadb" | grep -v grep
echo "=== LISTEN PORTS ===" && ss -tlnp 2>/dev/null || netstat -tlnp
echo "=== SYSTEMD ===" && systemctl list-units --type=service --state=running | grep -E "wa|mysql|mariadb|go"
echo "=== CRON ===" && crontab -l 2>/dev/null || echo "no crontab"
echo "=== MYSQL DB ===" && mysql -e "SHOW DATABASES" 2>/dev/null || echo "no mysql cli"
echo "=== MYSQL TABLES ===" && mysql -e "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema='db_wa_blast' OR table_schema='wa_assistant'" 2>/dev/null | head -30
echo "=== CHAT COUNT ===" && mysql -e "SELECT COUNT(*) as total_chats FROM db_wa_blast.chat_histories" 2>/dev/null || mysql -e "SELECT COUNT(*) as total_chats FROM wa_assistant.chat_histories" 2>/dev/null || echo "cannot count"
echo "=== DONE ==="
`;

const child = spawn('ssh', [
  '-tt',
  '-o', 'StrictHostKeyChecking=no',
  '-o', 'UserKnownHostsFile=/dev/null',
  '-o', 'PreferredAuthentications=password',
  '-o', 'PubkeyAuthentication=no',
  '-o', 'LogLevel=QUIET',
  '-o', 'NumberOfPasswordPrompts=1',
  HOST, CMD
], {
  stdio: ['pipe', 'pipe', 'pipe'],
  env: { ...process.env, SSH_ASKPASS: '', DISPLAY: '' }
});

let sentPass = false;
let out = '';

child.stdout.on('data', (d) => {
  out += d.toString();
  process.stdout.write(d.toString());
});

child.stderr.on('data', (d) => {
  const s = d.toString();
  out += s;
  // Don't show password prompt noise
  if (!sentPass && (s.includes('password') || s.includes('Password'))) {
    child.stdin.write(PASS + '\n');
    sentPass = true;
  } else if (!s.includes('password') && !s.includes('Password')) {
    process.stderr.write(s);
  }
});

child.on('error', (err) => {
  console.error('SSH Error:', err.message);
  process.exit(1);
});

child.on('close', (code) => {
  if (code !== 0) console.error('\nExit code:', code);
  process.exit(code || 0);
});

setTimeout(() => process.exit(1), 25000);
