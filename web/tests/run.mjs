import { spawn } from 'node:child_process';
import { once } from 'node:events';
const tests = ['dashboard-check', 'navigation-cache-check', 'completed-journey-check', 'layout-stability-check', 'motion-check', 'motion-actions-check', 'stream-check', 'feed-check', 'recognition-check', 'ux-check', 'bank-outcomes-check', 'manager-check', 'bank-check', 'product-check', 'auth-check', 'edit-check', 'confirm-check', 'sign-check', 'sign-errors', 'expiry-check', 'document-check', 'mode-check'];
for (const test of tests) {
  const server = spawn(process.execPath, ['node_modules/vite/bin/vite.js', '--mode', test === 'mode-check' ? 'development' : 'mock', '--host', '127.0.0.1', '--port', '5175', '--strictPort'], { env: { ...process.env, VITE_CACHE_DIR: 'node_modules/.vite-tests' }, stdio: ['ignore', 'pipe', 'pipe'] });
  let output = '';
  server.stdout.on('data', (chunk) => { output += chunk; });
  server.stderr.on('data', (chunk) => { output += chunk; });
  try {
    let ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      if (server.exitCode !== null) throw Error(output);
      try { if (output.includes('Local:')) { const response = await fetch('http://127.0.0.1:5175'); ready = response.ok; } } catch { /* сервер запускается */ }
      if (ready) break;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    if (!ready) throw Error('Не удалось запустить мок: ' + output);
    const check = spawn(process.execPath, [`tests/${test}.mjs`], { stdio: 'inherit' });
    const [code] = await once(check, 'exit');
    if (code !== 0) throw Error(`Проверка ${test} завершилась с кодом ${code}`);
  } finally {
    server.kill();
    if (server.exitCode === null) await once(server, 'exit');
  }
}














