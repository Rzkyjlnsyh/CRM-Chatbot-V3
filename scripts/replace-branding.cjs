const fs = require('fs');
const path = require('path');

const replacements = [
  ['ChatLoop', 'SlaluDiskon'],
  ['ChatLoopBot', 'SlaluDiskonBot'],
  ['NgertiKode.id | ChatLoop', 'SlaluDiskon'],
  ['NgertiKode.id', 'SlaluDiskon'],
  ['ngertikode.id', 'slaludiskon.com'],
  ['chatloop.id', 'slaludiskon.com'],
  ['CHATLOOP_', 'SLALUDISKON_'],
];

function walk(dir, exts) {
  const results = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules' || entry.name === '.git' || entry.name === 'data') continue;
      results.push(...walk(full, exts));
    } else if (exts.some(e => entry.name.endsWith(e))) {
      results.push(full);
    }
  }
  return results;
}

const exts = ['.go', '.tsx', '.ts', '.html', '.css', '.json', '.md', '.js', '.mjs'];
const files = walk('.', exts);

let totalChanged = 0;
for (const file of files) {
  let content = fs.readFileSync(file, 'utf8');
  let changed = false;
  for (const [from, to] of replacements) {
    if (content.includes(from)) {
      content = content.split(from).join(to);
      changed = true;
    }
  }
  if (changed) {
    fs.writeFileSync(file, content);
    totalChanged++;
    console.log('OK:', file);
  }
}

console.log('\nTotal files changed:', totalChanged);
