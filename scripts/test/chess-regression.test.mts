import path from 'node:path';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const chess = path.resolve(process.env.NETLIB_CHESS_WORKSPACE!);
const root = fileURLToPath(new URL('../..', import.meta.url));
const signaling = 'ws://127.0.0.1:18080/v0/signaling';
const fromChess = (file: string) => import(pathToFileURL(path.join(chess, file)).href);
const { test, expect } = await fromChess('e2e/fixtures/test.ts');
const { ApiMock } = await fromChess('e2e/helpers/api-mock.ts');
const { BasePage } = await fromChess('e2e/pages/BasePage.ts');
const { privateLinkOnlineTogetherScenario } = await fromChess('e2e/projects/shared-scenarios/private-link-online-together.scenario.ts');
const requireChess = createRequire(path.join(chess, 'packages/peer/package.json'));
const version = requireChess('@poki/netlib/package.json').version;
if (version !== '0.0.16') throw new Error(`Ожидался текущий клиент 0.0.16, получен ${version}`);

// Моки относятся только к внешней платформе; настоящий сигналинг не подменяется.
const originalApiInit = ApiMock.init.bind(ApiMock);
ApiMock.init = (page: any, params: any) => originalApiInit(page, { ...params, api: params?.api ?? true, ws: false });

let current: any;
const originalBeforeGoto = BasePage.prototype.beforeGoto;
BasePage.prototype.beforeGoto = async function (...args: any[]) {
  await originalBeforeGoto.apply(this, args);
  await this.page.addInitScript(url => {
    const descriptor = Object.getOwnPropertyDescriptor(window, 'smGameSdk');
    const configure = (sdk: any) => {
      if (!sdk || sdk.__netlibLocalFlags) return;
      sdk.mainSdkPromise = sdk.mainSdkPromise.then((result: any) => {
        // Данные игроков обслуживает существующий ApiMock, без обращений к GamePush.
        const gp = (window as any).gp;
        gp.variables.state.USE_EXTERNAL_API = '1';
        return result;
      });
      const original = sdk.getPlatformFlags.bind(sdk);
      sdk.getPlatformFlags = async (...args: any[]) => ({ ...await original(...args), netlib_host: url });
      sdk.__netlibLocalFlags = true;
    };
    configure((window as any).smGameSdk);
    Object.defineProperty(window, 'smGameSdk', {
      configurable: true,
      get: () => descriptor?.get?.call(window),
      set: sdk => { configure(sdk); descriptor?.set?.call(window, sdk); },
    });
  }, signaling);
};
const originalGoto = BasePage.prototype.goto;
BasePage.prototype.goto = async function (params: any) {
  const page = this.page;
  const record = { page, sockets: 0, pingSocket: 0 };
  current.pages.push(record);
  await page.context().routeWebSocket(url => url.hostname !== '127.0.0.1', socket => {
    if (new URL(socket.url()).pathname === '/v0/signaling') current.unexpected.push(socket.url());
    socket.close({ code: 1008, reason: 'Тест разрешает только локальный сигналинг' });
  });
  await page.context().route(url => url.hostname !== '127.0.0.1', route => {
    if (['GET', 'HEAD', 'OPTIONS'].includes(route.request().method())) return route.continue();
    return route.fulfill({ status: 204 });
  });
  page.on('websocket', socket => {
    if (socket.url() !== signaling) return;
    const index = ++record.sockets;
    socket.on('framereceived', frame => {
      try {
        if (JSON.parse(String(frame.payload)).type === 'ping') record.pingSocket = index;
      } catch {}
    });
  });
  if (current.phase === 'connecting') {
    await page.exposeFunction('__netlibBeforeSdp', async () => {
      if (current.restarts > 0) return;
      await current.restart('connecting');
    });
    await page.addInitScript(() => {
      const original = RTCPeerConnection.prototype.setLocalDescription;
      RTCPeerConnection.prototype.setLocalDescription = async function (...args) {
        await (window as any).__netlibBeforeSdp();
        return original.apply(this, args);
      };
    });
  }
  await originalGoto.call(this, params);
  const preconditions = await page.evaluate(() => {
    const state = (window as any).store();
    return { cash: state.platform.playerCash, bet: state.config.onlineBetSize, signaling: state.flags.flags.netlib_host };
  });
  expect(preconditions.signaling).toBe(signaling);
  expect(preconditions.cash).toBeGreaterThanOrEqual(preconditions.bet);
};

for (const phase of ['waiting', 'connecting', 'active']) {
  test(`Настоящие шахматы 0.0.16: рестарт сигналинга — ${phase}`, async ({ basePage, browser, gameHelper, modalHelper, navigationHelper, page }, testInfo) => {
    current = { phase, pages: [], unexpected: [], restarts: 0 };
    current.restart = async (reason: string) => {
      current.restarts++;
      const peers = current.pages.filter((item: any) => item.sockets > 0);
      expect(peers.length).toBeGreaterThan(0);
      const before = peers.map((item: any) => item.sockets);
      execFileSync('docker', ['compose', '-p', 'netlib-regression', '-f', path.join(root, 'scripts/test/compose.yml'), 'restart', 'netlib'], { cwd: root, timeout: 30000, stdio: 'pipe' });
      await expect.poll(() => peers.every((item: any, index: number) => item.sockets > before[index] && item.pingSocket === item.sockets), { timeout: 30000 }).toBe(true);
      console.log(`Сигналинг восстановлен: ${reason}, клиентов: ${peers.length}`);
    };
    if (phase === 'waiting') {
      const status = modalHelper.onlineTogetherGameStatus;
      const originalInvite = status.getInviteLinkParams.bind(status);
      status.getInviteLinkParams = async () => {
        const params = await originalInvite();
        await current.restart('waiting');
        return params;
      };
    }
    const originalMove = gameHelper.chess.move.bind(gameHelper.chess);
    const hostGame = {
      move: async (from: string, to: string) => {
        await originalMove(from, to);
        if (phase !== 'active') return;
        const pages = current.pages.map((item: any) => item.page);
        const length = await page.evaluate(() => (window as any).store().board.gameEngine.history().length);
        await expect.poll(() => Promise.all(pages.map((p: any) => p.evaluate(() => (window as any).store().board.gameEngine.history().length)))).toEqual(pages.map(() => length));
        const before = await Promise.all(pages.map((p: any) => p.evaluate(() => ({ match: (window as any).store().p2p.currentMatchId, history: (window as any).store().board.gameEngine.history() }))));
        await current.restart('active');
        const after = await Promise.all(pages.map((p: any) => p.evaluate(() => ({ match: (window as any).store().p2p.currentMatchId, history: (window as any).store().board.gameEngine.history() }))));
        expect(after).toEqual(before);
      },
    };
    try {
      await privateLinkOnlineTogetherScenario({ basePage, browser, modalHelper, navigationHelper, page }, hostGame, { guestGame: 'chess', hostMove: ['e2', 'e4'], guestMove: ['e7', 'e5'] });
      expect(current.unexpected).toEqual([]);
      expect(current.restarts).toBe(1);
    } finally {
      await testInfo.attach('real-signaling', { body: JSON.stringify({ phase, version, signaling, baseline: process.env.NETLIB_CHESS_BASELINE === '1', restarts: current.restarts, unexpected: current.unexpected, clients: current.pages.map(({ sockets, pingSocket }: any) => ({ sockets, pingSocket })) }, null, 2), contentType: 'application/json' });
      for (const context of browser.contexts()) await context.close();
    }
  });
}
