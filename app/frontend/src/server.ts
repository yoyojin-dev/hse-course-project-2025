import express from 'express';
import { createProxyMiddleware } from 'http-proxy-middleware';
import history from 'connect-history-api-fallback';
import path from 'path';
import http from 'http';
import fs from 'fs';

const PORT = process.env.PORT ? parseInt(process.env.PORT, 10) : 3000;
const FRONTEND_STATIC = path.join(__dirname, '..', 'static');

const app = express();
app.use(express.json());

// --- Simple helper to GET JSON from backend without extra deps
function getJsonFromBackend(pathname: string): Promise<any> {
  return new Promise((resolve, reject) => {
    const options: http.RequestOptions = {
      hostname: 'backend',
      port: 8080,
      path: pathname,
      method: 'GET',
      headers: {
        'Accept': 'application/json'
      }
    };

    const req = http.request(options, (res) => {
      let data = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        if (!data) return resolve(null);
        try {
          const parsed = JSON.parse(data);
          resolve(parsed);
        } catch (e) {
          // If response is not JSON, return raw text
          resolve(data);
        }
      });
    });

    req.on('error', (err) => reject(err));
    req.end();
  });
}
app.route('/')
    .get(async (req, res) => {
        res.sendFile(path.join(FRONTEND_STATIC, 'index.html'), (err) => {
            if (err) {
                console.error('SendFile / error:', err);
                res.status((err as any)?.status || 500).end();
            }
        });
    });

app.route('/joining/:gamecode')
    .get(async (req, res) => {
        res.sendFile(path.join(FRONTEND_STATIC, 'joining.html'), (err) => {
            if (err) {
                console.error('SendFile /joining/:gamecode error:', err);
                res.status((err as any)?.status || 500).end();
            }
        });
    })

app.route('/create')
  .get((req, res) => {
    // Delegate creation to backend: redirect client to /api/create
    // nginx proxies /api/* to backend, so backend will perform creation and redirect as needed
    res.redirect(302, '/api/create');
  })

function safeHtml(s: string): string {
    const map: Record<string,string> = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
    };
    return String(s).replace(/[&<>"']/g, (ch) => map[ch]);
}

app.get('/created/:gamecode', (req, res) => {
  const code = req.params.gamecode || '';
  const safe = safeHtml(code);

  const filePath = path.join(FRONTEND_STATIC, 'createdGame.html');
  try {
    let template = fs.readFileSync(filePath, 'utf8');
    // Replace {{ game_code }} occurrences
    template = template.replace(/{{\s*game_code\s*}}/g, () => safe);

    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.send(template);
  } catch (err) {
    console.error('Error reading template', filePath, err);
    res.status(500).send('Server error');
  }
});


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


// --- Serve static files (do not auto-serve index for '/').
app.use(express.static(FRONTEND_STATIC, { index: false }));

// --- Start server
app.listen(PORT, '0.0.0.0', () => {
  console.log(`Frontend proxy listening on http://0.0.0.0:${PORT}`);
});
