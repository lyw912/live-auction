const apiURL = process.env.TOXIPROXY_API || 'http://127.0.0.1:8474';

async function request(path, options = {}) {
  const response = await fetch(`${apiURL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
  });
  if (!response.ok && response.status !== 409 && response.status !== 404) {
    throw new Error(`${options.method ?? 'GET'} ${path} failed: ${response.status} ${await response.text()}`);
  }
  if (response.status === 204 || response.status === 404) return null;
  return response.json();
}

export async function resetToxiproxy() {
  const proxies = await request('/proxies');
  for (const name of Object.keys(proxies ?? {})) {
    await request(`/proxies/${encodeURIComponent(name)}`, { method: 'DELETE' });
  }
}

export async function listProxies() {
  return request('/proxies');
}

export async function getProxy(proxyName) {
  return request(`/proxies/${encodeURIComponent(proxyName)}`);
}

export async function createProxy(proxy) {
  const existing = await request(`/proxies/${encodeURIComponent(proxy.name)}`);
  if (existing) {
    await request(`/proxies/${encodeURIComponent(proxy.name)}`, { method: 'DELETE' });
  }
  await request('/proxies', {
    method: 'POST',
    body: JSON.stringify(proxy),
  });
}

export async function addToxic(proxyName, toxic) {
  await request(`/proxies/${encodeURIComponent(proxyName)}/toxics`, {
    method: 'POST',
    body: JSON.stringify(toxic),
  });
}

export async function removeToxic(proxyName, toxicName) {
  await request(`/proxies/${encodeURIComponent(proxyName)}/toxics/${encodeURIComponent(toxicName)}`, {
    method: 'DELETE',
  });
}
