// Проверка опубликованных клиентов с настоящими WebRTC, Nginx и PostgreSQL.
const assert = require('node:assert/strict');
const { createHash, randomUUID } = require('node:crypto');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs/promises');
const http = require('node:http');
const path = require('node:path');

const root = path.resolve(__dirname, '../..');
const cache = path.join(root, 'node_modules/.cache/netlib-regression');
const versions = {
  '0.0.16': 'sha512-EARw9qzHb1dk2dA2+89t8eetrOUlt0FJuVx65v1Md+EiFPgd3aP+QbOrDtTptMuW8kNhxcmCi+gzH+yrhP6oAQ==',
  '0.0.20': 'sha512-x23mkcWwXU2WyyYUKOVyL2IdZGg8u7XLsuxMsxS0x2HNSh5v+5GgcjZ0llr8CwsQXYTtO8XJnCquFbyyemVpVg==',
};
const signaling = process.env.NETLIB_TEST_SIGNALING_URL || 'ws://127.0.0.1:18080/v0/signaling';
const endpoint = new URL(signaling);
assert(['127.0.0.1', 'localhost', '[::1]'].includes(endpoint.hostname), 'тест работает только с локальным сервером');
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || '@playwright/test');

async function clients() {
  const output = {};
  for (const [version, integrity] of Object.entries(versions)) {
    const directory = path.join(cache, version);
    const tarball = path.join(directory, 'package.tgz');
    await fs.mkdir(directory, { recursive: true });
    let data;
    try { data = await fs.readFile(tarball); } catch (error) {
      if (error.code !== 'ENOENT') throw error;
      const response = await fetch(`https://registry.npmjs.org/@poki/netlib/-/netlib-${version}.tgz`);
      assert(response.ok, `npm: ${response.status}`);
      data = Buffer.from(await response.arrayBuffer());
    }
    assert.equal(`sha512-${createHash('sha512').update(data).digest('base64')}`, integrity, `целостность ${version}`);
    await fs.writeFile(tarball, data);
    execFileSync('tar', ['-xzf', tarball, '-C', directory], { stdio: 'pipe', windowsHide: true });
    const metadata = JSON.parse(await fs.readFile(path.join(directory, 'package/package.json'), 'utf8'));
    assert.equal(metadata.name, '@poki/netlib');
    assert.equal(metadata.version, version);
    output[version] = {
      main: await fs.readFile(path.join(directory, 'package/dist/netlib.js'), 'utf8'),
      legacy: await fs.readFile(path.join(directory, 'package/dist/legacy.js'), 'utf8'),
    };
  }
  return output;
}

async function createPage(browser, baseURL, version, bundle, game) {
  const context = await browser.newContext();
  const page = await context.newPage();
  page.on('pageerror', error => console.error(`BROWSER ${version}/${bundle}: ${error.message}`));
  page.setDefaultTimeout(30000);
  await page.goto(`${baseURL}/?version=${version}&bundle=${bundle}`);
  await page.evaluate(async ({ signaling, game }) => {
    const n = window.network = new window.netlib.Network(game, {
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
    }, signaling);
    window.observed = { messages: [], connected: [], errors: [], signalingReconnects: 0 };
    n.on('message', (peer, channel, data) => {
      const text = typeof data === 'string' ? data : new TextDecoder().decode(data);
      window.observed.messages.push({ peer: peer.id, channel, text });
    });
    n.on('connected', peer => window.observed.connected.push(peer.id));
    n.on('rtcerror', error => window.observed.errors.push(String(error?.message || error)));
    n.on('signalingerror', error => window.observed.errors.push(String(error?.message || error)));
    n.on('signalingreconnected', () => window.observed.signalingReconnects++);
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('нет ready от сигналинга за 30 секунд')), 30000);
      n.once('ready', () => { clearTimeout(timeout); resolve(); });
    });
  }, { signaling, game });
  return { page, context };
}

async function exchange(host, guest, label) {
  const ids = await Promise.all([host, guest].map(page => page.evaluate(() => window.network.id)));
  await Promise.all([host, guest].map(page => page.waitForFunction(() => window.network.size === 1 && [...window.network.peers.values()].every(peer => peer.opened))));
  for (const [sender, receiver, recipient, direction] of [[host, guest, ids[1], 'host'], [guest, host, ids[0], 'guest']]) {
    for (const channel of ['reliable', 'unreliable']) {
      const text = `${label}:${direction}:${channel}`;
      await sender.evaluate(({ channel, recipient, text }) => window.network.send(channel, recipient, text), { channel, recipient, text });
      await receiver.waitForFunction(({ channel, text }) => window.observed.messages.some(message => message.channel === channel && message.text === text), { channel, text });
      assert.equal(await receiver.evaluate(text => window.observed.messages.filter(message => message.text === text).length, text), 1);
    }
  }
  for (const page of [host, guest]) {
    assert.deepEqual(await page.evaluate(() => Object.fromEntries(Object.entries([...window.network.peers.values()][0].channels).map(([label, channel]) => [label, channel.id]))), { reliable: 0, unreliable: 1, control: 2 });
    await page.waitForFunction(() => Number.isFinite([...window.network.peers.values()][0].latency.average) && [...window.network.peers.values()][0].latency.average > 0);
  }
}

function restartServer() {
  execFileSync('docker', ['compose', '-p', 'netlib-regression', '-f', path.join(__dirname, 'compose.yml'), 'restart', 'netlib'], { stdio: 'pipe', windowsHide: true, timeout: 45000 });
}

async function main() {
  const source = await clients();
  const emitter = await fs.readFile(require.resolve('eventemitter3'), 'utf8');
  const server = http.createServer((request, response) => {
    const url = new URL(request.url, 'http://localhost');
    const version = url.searchParams.get('version') || '0.0.20';
    const bundle = url.searchParams.get('bundle') || 'main';
    if (!source[version] || !['main', 'legacy'].includes(bundle)) { response.writeHead(404).end(); return; }
    if (url.pathname === '/client.js') {
      response.setHeader('content-type', 'text/javascript');
      if (bundle === 'legacy') response.end(source[version].legacy);
      else response.end(`(() => { const emitterModule = {exports:{}}; ((module,exports)=>{${emitter}\n})(emitterModule,emitterModule.exports); const clientModule = {exports:{}}; ((module,exports,require,process)=>{${source[version].main}\n})(clientModule,clientModule.exports,name=>{if(name==='eventemitter3')return emitterModule.exports;throw new Error(name)},{env:{NODE_ENV:'production'}}); window.netlib=clientModule.exports; })();`);
      return;
    }
    response.setHeader('content-type', 'text/html');
    response.end(`<!doctype html><meta charset="utf-8"><title>netlib regression</title><script src="/client.js?version=${version}&bundle=${bundle}"></script>`);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  let browser;
  const baseURL = `http://127.0.0.1:${server.address().port}`;
  const evidence = { server: process.env.NETLIB_TEST_IMAGE || 'netlib:69212489-candidate', signaling, completed: false, results: [] };
  try {
    browser = await chromium.launch({ headless: true, args: ['--mute-audio'] });
    for (const [hostVersion, guestVersion, bundle] of [
      ['0.0.16', '0.0.16', 'main'],
      ['0.0.20', '0.0.20', 'main'],
      ['0.0.16', '0.0.20', 'main'],
      ['0.0.20', '0.0.16', 'main'],
      ['0.0.20', '0.0.20', 'legacy'],
    ]) {
      const game = randomUUID();
      console.log(`RUN ${hostVersion}/${guestVersion}/${bundle}`);
      const host = await createPage(browser, baseURL, hostVersion, bundle, game);
      const guest = await createPage(browser, baseURL, guestVersion, bundle, game);
      const label = `${hostVersion}/${guestVersion}/${bundle}`;
      try {
        const code = await host.page.evaluate(() => window.network.create({ public: true, maxPlayers: 4, customData: { regression: true } }));
        assert(code);
        const listed = await guest.page.evaluate(() => window.network.list());
        assert(listed.some(lobby => lobby.code === code && lobby.customData.regression));
        await guest.page.evaluate(code => window.network.join(code), code);
        await exchange(host.page, guest.page, label);
        assert.equal(await host.page.evaluate(() => window.observed.connected.length), 1);
        assert.equal(await guest.page.evaluate(() => window.observed.connected.length), 1);
        assert.deepEqual(await host.page.evaluate(() => window.observed.errors), []);
        assert.deepEqual(await guest.page.evaluate(() => window.observed.errors), []);
        if (process.env.NETLIB_TEST_RESTART === '1' && hostVersion === '0.0.16' && guestVersion === '0.0.20') {
          const before = await Promise.all([host.page, guest.page].map(page => page.evaluate(() => ({ id: window.network.id, lobby: window.network.currentLobby }))));
          restartServer();
          await Promise.all([host.page, guest.page].map(page => page.waitForFunction(() => window.observed.signalingReconnects > 0)));
          const after = await Promise.all([host.page, guest.page].map(page => page.evaluate(() => ({ id: window.network.id, lobby: window.network.currentLobby }))));
          assert.deepEqual(after, before);
          await exchange(host.page, guest.page, `${label}:restart`);
          evidence.results.push({ scenario: 'signaling-restart', label, status: 'passed' });
        }
        evidence.results.push({ scenario: 'create-list-join-messages-control', label, status: 'passed' });
        console.log(`PASS ${label}`);
      } finally {
        await Promise.allSettled([host.page, guest.page].map(page => page.evaluate(() => window.network.close('regression finished'))));
        await host.context.close();
        await guest.context.close();
      }
    }
    if (process.env.NETLIB_TEST_CLOSE_QUEUE === '1') {
      const client = await createPage(browser, baseURL, '0.0.20', 'main', randomUUID());
      try {
        const state = await client.page.evaluate(async () => {
          const n = window.network;
          const signaling = n.signaling;
          await signaling.messageQueue;
          let release;
          signaling.messageQueue = new Promise(resolve => { release = resolve; });
          signaling.enqueueSignalingMessage(JSON.stringify({ type: 'connect', id: 'queued-before-close', polite: true }));
          n.close('cancel pending connection');
          release();
          await signaling.messageQueue;
          const result = { closing: n.closing, peers: n.size };
          n.peers.forEach(peer => peer.close('test cleanup'));
          return result;
        });
        evidence.results.push({ scenario: 'close-with-queued-connect', state, status: state.closing && state.peers === 0 ? 'passed' : 'failed' });
        assert.deepEqual(state, { closing: true, peers: 0 }, 'SC-05: после закрытия не должен появляться новый Peer');
      } finally {
        await client.context.close();
      }
    }
    evidence.completed = true;
  } finally {
    await browser?.close();
    await new Promise(resolve => server.close(resolve));
    await fs.writeFile(path.join(cache, `browser-${Date.now()}.json`), JSON.stringify(evidence, null, 2));
  }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
