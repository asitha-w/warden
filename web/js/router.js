// Hash routes. route() resolves the hash against the setup state so a user
// without a tenant always lands in setup.
import { store } from './store.js';
export function route() {
  const s = store.state;
  const h = location.hash.replace(/^#/, '') || 'findings';
  if (!s || !s.user) return 'login';
  if (!s.tenant) return h.indexOf('setup') === 0 || h.indexOf('docs') === 0 ? h : 'setup-tenant';
  if (h === 'login') return 'findings';
  return h;
}
export function go(h) { location.hash = '#' + h; }
