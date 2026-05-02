import express from 'express';
import { createProxyMiddleware } from 'http-proxy-middleware';
import history from 'connect-history-api-fallback';
import path from 'path';

const PORT = process.env.PORT ? parseInt(process.env.PORT, 10) : 3000;
const FRONTEND_STATIC = path.join(__dirname, '..', 'static');

const app = express();
app.use(express.json());

app.route('/create')
  .get((req, res) => {
    // Delegate creation to backend: redirect client to /api/create
    // nginx proxies /api/* to backend, so backend will perform creation and redirect as needed
    res.redirect(302, '/api/create');
  })


function proxyOnError(err: any, req: any, res: any) {
    console.error('Proxy error for', req.url, err && err.message);
    if (!res.headersSent) {
        res.writeHead(502, { 'Content-Type': 'text/plain; charset=utf-8' });
    }
    try { res.end('Bad gateway'); } catch (e) { /* ignore */ }
}

// --- Proxy /api/ to backend
app.use('/api', createProxyMiddleware({
  target: 'http://backend:8080',
  changeOrigin: true,
  ws: true,
  pathRewrite: {
    '^/api': '/api'
  },
  logLevel: 'warn',
  onError: proxyOnError
}));

// --- Proxy /ws/ to backend (WebSocket)
app.use('/ws', createProxyMiddleware({
  target: 'http://backend:8080',
  changeOrigin: true,
  ws: true,
  logLevel: 'warn',
  onProxyReqWs: (proxyReq, req, socket, options, head) => {
    // Ensure headers for WebSocket upgrade are preserved
    const upgradeHeader = (req.headers['upgrade'] || '').toString();
    if (upgradeHeader) {
      proxyReq.setHeader('Upgrade', upgradeHeader);
    }
    const connectionHeader = (req.headers['connection'] || '').toString();
    if (connectionHeader) {
      proxyReq.setHeader('Connection', connectionHeader);
    } else {
      proxyReq.setHeader('Connection', 'upgrade');
    }
  }
}));

// --- SPA fallback middleware
app.use(history({
  rewrites: [
    { from: /^\/api\//, to: (context: any) => context.parsedUrl.path },
    { from: /^\/ws\//, to: (context: any) => context.parsedUrl.path }
  ],
}));


// --- Serve static files
app.use(express.static(FRONTEND_STATIC));

// --- Start server
app.listen(PORT, '0.0.0.0', () => {
  console.log(`Frontend proxy listening on http://0.0.0.0:${PORT}`);
});
