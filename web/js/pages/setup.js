import { el, Button } from '../ds.js';
import { icon } from '../icons.js';
import { store, sel } from '../store.js';
import { connChip } from '../connections.js';
import { api } from '../api.js';
import { go } from '../router.js';
import { toast } from '../shell.js';

export function pageLogin() {
  const email = el('input', { id: 'login-email', type: 'email', placeholder: 'you@company.example', autocomplete: 'username' });
  const pw = el('input', { id: 'login-pw', type: 'password', placeholder: 'Password', autocomplete: 'current-password' });
  const signIn = (name) => api.login(email.value || 'asitha@company.example', name).catch((e) => toast(e.message));
  const form = el('form', { class: 'form', onSubmit: (e) => { e.preventDefault(); signIn(); } }, [
    el('div', { class: 'field' }, [el('label', { for: 'login-email', text: 'Work email' }), email]),
    el('div', { class: 'field' }, [el('label', { for: 'login-pw', text: 'Password' }), pw]),
    Button({ variant: 'primary', label: 'Sign in', type: 'submit' }),
  ]);
  return el('div', { class: 'center' }, [el('div', { class: 'login' }, [
    el('div', { class: 'brand' }, [el('i', null, [icon('warden')]), 'Warden']),
    el('p', { class: 'muted', text: 'Reliability operations for teams without a full-time SRE. Sign in to your workspace.' }),
    form, el('div', { class: 'hr' }),
    Button({ label: 'Continue with Microsoft Entra', onClick: () => signIn('asitha') }),
    el('p', { class: 'sm muted', text: 'Prototype. Any email and password sign you in. Nothing leaves this server.' }),
  ])]);
}

// docLinks points at the documentation from the setup pages: the model, then the exact steps per provider.
export function docLinks() {
  return el('p', { class: 'sm muted doclinks' }, ['Before you connect: ', el('a', { href: '#docs/model', text: 'how levels and cells work' }), ' · ', el('a', { href: '#docs/azure', text: 'Azure, step by step' }), ' · ', el('a', { href: '#docs/upcloud', text: 'UpCloud, step by step' }), ' · ', el('a', { href: '#docs/howto', text: 'all how-tos' })]);
}

function stepsBar(n) {
  const names = ['Sign in', 'Create tenant', 'Connect, onboard and raise cells in the console'];
  return el('div', { class: 'steps' }, names.map((s, i) => el('span', { class: i < n ? 'done' : (i === n ? '' : 'muted') }, [i < n ? '✓ ' : (i + 1) + '. ', i === n ? el('b', { text: s }) : s])));
}

export function pageSetupTenant() {
  const name = el('input', { id: 'tenant-name', placeholder: 'Shop AS', value: 'Shop AS' });
  const env = el('select', { id: 'tenant-env' }, [el('option', { text: 'production' }), el('option', { text: 'staging' })]);
  const region = el('select', { id: 'tenant-region' }, [el('option', { text: 'EU, Finland (UpCloud fi-hel2)' }), el('option', { text: 'EU, Netherlands (Azure westeurope)' })]);
  const form = el('form', { class: 'form', onSubmit: (e) => { e.preventDefault(); api.createTenant({ name: name.value, env: env.value, region: region.value }).then(() => go('findings')).catch((x) => toast(x.message)); } }, [
    el('div', { class: 'field' }, [el('label', { for: 'tenant-name', text: 'Tenant name' }), name]),
    el('div', { class: 'field' }, [el('label', { for: 'tenant-env', text: 'First environment' }), env]),
    el('div', { class: 'field' }, [el('label', { for: 'tenant-region', text: 'Where the Warden cell runs' }), region]),
    el('p', { class: 'sm muted', text: 'The tenant gets its own runtime: isolated workers, its own credentials, one static egress address you can allowlist.' }),
    el('p', { class: 'sm muted' }, ['New to Warden? Read ', el('a', { href: '#docs/model', text: 'how Warden reaches your infrastructure' }), ' first.']),
    Button({ variant: 'primary', label: 'Create tenant', type: 'submit' }),
  ]);
  return el('div', { class: 'page' }, [stepsBar(1), el('h1', { class: 'display', text: 'Create your tenant' }), el('div', { class: 'card lg' }, [form])]);
}

