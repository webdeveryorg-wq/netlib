const path = require('node:path');

const chess = process.env.NETLIB_CHESS_WORKSPACE;
if (!chess) throw new Error('Укажите NETLIB_CHESS_WORKSPACE');

module.exports = {
  testDir: __dirname,
  testMatch: 'chess-regression.test.mts',
  tsconfig: path.join(chess, 'tsconfig.json'),
  workers: 1,
  retries: 0,
  timeout: 150000,
  reporter: [['list']],
  outputDir: path.resolve(__dirname, '../../node_modules/.cache/netlib-regression', process.env.NETLIB_CHESS_BASELINE === '1' ? 'chess-baseline' : 'chess-results'),
  use: {
    baseURL: 'http://127.0.0.1:3196',
    headless: true,
    locale: 'ru-RU',
    launchOptions: { args: ['--mute-audio'] },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: `node "${path.join(chess, 'node_modules/vite/bin/vite.js')}" --host 127.0.0.1 --port 3196 --strictPort`,
    cwd: path.join(chess, 'games/chess'),
    url: 'http://127.0.0.1:3196',
    timeout: 180000,
    reuseExistingServer: false,
    stdout: 'ignore',
    stderr: 'pipe',
  },
};
