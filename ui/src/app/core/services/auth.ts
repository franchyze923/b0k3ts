import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

export type AuthenticateResponse = {
  authenticated: boolean;
  user_info: User | null;
};

export type LocalLoginRequest = {
  username: string;
  password: string;
};

type StartLoginResponse = {
  registrationUrl?: string;
  registration_url?: string;
  url?: string;
};

type LocalLoginResponse =
  | StartLoginResponse
  | {
      authenticated?: boolean;
      token?: string;
      username?: string;
    };

@Injectable({
  providedIn: 'root',
})
export class Auth {
  private readonly apiBase = '';

  private readonly storageTokenKey = 'oidc.token';
  private readonly storageStateKey = 'oidc.state';
  private readonly storageAuthHintKey = 'auth.hint'; // 'oidc' | 'local'

  constructor(private readonly http: HttpClient) {}

  generateToken(byteLength = 32): string {
    const bytes = new Uint8Array(byteLength);
    crypto.getRandomValues(bytes);
    return this.base64UrlEncode(bytes);
  }

  async startLogin(): Promise<{ registrationUrl: string }> {
    const redirectUri = new URL('/oidc/callback', globalThis.location.origin).toString();

    const state = this.generateToken(32);
    sessionStorage.setItem(this.storageStateKey, state);

    const url = `${this.apiBase}/api/v1/oidc/login`;

    let res: StartLoginResponse;
    try {
      res = await firstValueFrom(
        this.http.get<StartLoginResponse>(url, {
          params: {
            redirect_uri: redirectUri,
            state,
          },
        }),
      );
    } catch {
      throw new Error('Login start request failed.');
    }

    const registrationUrl = res.registrationUrl ?? res.registration_url ?? res.url;
    if (!registrationUrl) {
      throw new Error('Backend did not provide a registration URL.');
    }

    return { registrationUrl };
  }

  async startLocalLogin(
    req: LocalLoginRequest,
  ): Promise<{ registrationUrl?: string; token?: string }> {
    const redirectUri = new URL('/local/callback', globalThis.location.origin).toString();

    const state = this.generateToken(32);
    sessionStorage.setItem(this.storageStateKey, state);

    const url = `${this.apiBase}/api/v1/local/login`;

    let res: LocalLoginResponse;
    try {
      res = await firstValueFrom(
        this.http.post<LocalLoginResponse>(url, req, {
          params: {
            redirect_uri: redirectUri,
            state,
          },
        }),
      );
    } catch {
      throw new Error('Local login start request failed.');
    }

    // 1) Direct token response (your current backend behavior)
    const token = (res as any)?.token as string | undefined;
    const authenticated = (res as any)?.authenticated as boolean | undefined;
    if (token && authenticated !== false) {
      return { token };
    }

    // 2) Redirect response (OIDC-like)
    const registrationUrl =
      (res as any)?.registrationUrl ?? (res as any)?.registration_url ?? (res as any)?.url;

    if (!registrationUrl) {
      throw new Error('Backend did not provide a token or a registration URL.');
    }

    return { registrationUrl };
  }

  async authenticateAny(token?: string): Promise<AuthenticateResponse> {
    const t = token ?? this.getToken();
    if (!t) return { authenticated: false, user_info: null };

    // 1) Prefer what succeeded previously to avoid “probing” 401s
    const hint = this.getAuthHint();
    if (hint === 'oidc') {
      const oidc = await this.authenticate(t);
      if (oidc.authenticated) return oidc;
      const local = await this.authenticateLocal(t);
      return local;
    }
    if (hint === 'local') {
      const local = await this.authenticateLocal(t);
      if (local.authenticated) return local;
      const oidc = await this.authenticate(t);
      return oidc;
    }

    // 2) No hint yet: try to guess once, then fall back
    const order = this.guessAuthOrderFromToken(t);
    if (order === 'oidc-first') {
      const oidc = await this.authenticate(t);
      if (oidc.authenticated) return oidc;
      return await this.authenticateLocal(t);
    } else {
      const local = await this.authenticateLocal(t);
      if (local.authenticated) return local;
      return await this.authenticate(t);
    }
  }

  setToken(token: string): void {
    sessionStorage.setItem(this.storageTokenKey, token);
  }

  getToken(): string | null {
    return sessionStorage.getItem(this.storageTokenKey);
  }

  clearToken(): void {
    sessionStorage.removeItem(this.storageTokenKey);
    sessionStorage.removeItem(this.storageStateKey);
    sessionStorage.removeItem(this.storageAuthHintKey);
  }

  async authenticate(token?: string): Promise<AuthenticateResponse> {
    const t = token ?? this.getToken();
    if (!t) return { authenticated: false, user_info: null };

    const url = `${this.apiBase}/api/v1/oidc/authenticate`;
    const headers = new HttpHeaders({ Authorization: `${t}` });

    try {
      const res = await firstValueFrom(this.http.post<AuthenticateResponse>(url, {}, { headers }));
      if (res.authenticated) this.setAuthHint('oidc');
      return res;
    } catch {
      return { authenticated: false, user_info: null };
    }
  }

  async authenticateLocal(token?: string): Promise<AuthenticateResponse> {
    const t = token ?? this.getToken();
    if (!t) return { authenticated: false, user_info: null };

    const url = `${this.apiBase}/api/v1/local/authenticate`;
    const headers = new HttpHeaders({ Authorization: `Bearer ${t}` });

    try {
      const res = await firstValueFrom(this.http.post<AuthenticateResponse>(url, {}, { headers }));
      if (res.authenticated) this.setAuthHint('local');
      return res;
    } catch {
      return { authenticated: false, user_info: null };
    }
  }

  private setAuthHint(hint: 'oidc' | 'local'): void {
    sessionStorage.setItem(this.storageAuthHintKey, hint);
  }

  private getAuthHint(): 'oidc' | 'local' | null {
    const v = sessionStorage.getItem(this.storageAuthHintKey);
    return v === 'oidc' || v === 'local' ? v : null;
  }

  private guessAuthOrderFromToken(token: string): 'oidc-first' | 'local-first' {
    // Heuristic: JWT-ish tokens are more likely to be OIDC.
    // (Doesn’t verify signature; just reduces unnecessary probes on first run.)
    const parts = token.split('.');
    if (parts.length !== 3) return 'local-first';

    try {
      const payloadJson = this.base64UrlDecodeToString(parts[1]);
      const payload = JSON.parse(payloadJson) as Record<string, unknown>;
      if (
        typeof payload['iss'] === 'string' ||
        typeof payload['aud'] === 'string' ||
        Array.isArray(payload['aud'])
      ) {
        return 'oidc-first';
      }
      return 'oidc-first'; // still JWT-looking
    } catch {
      return 'local-first';
    }
  }

  private base64UrlDecodeToString(b64url: string): string {
    const padLen = (4 - (b64url.length % 4)) % 4;
    const padded = b64url.replaceAll('-', '+').replaceAll('_', '/') + '='.repeat(padLen);
    return atob(padded);
  }

  private base64UrlEncode(bytes: Uint8Array): string {
    let binary = '';
    for (const b of bytes) binary += String.fromCodePoint(b);

    const base64 = btoa(binary);

    // Avoid regex (linear-time string ops only)
    let out = base64.replaceAll('+', '-').replaceAll('/', '_');
    while (out.endsWith('=')) out = out.slice(0, -1);

    return out;
  }
}

export type User = {
  id: string;
  email: string;
  name: string;
  preferred_username: string;
  groups: string[];
};
