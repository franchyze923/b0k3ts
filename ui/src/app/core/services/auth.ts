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

  constructor(private readonly http: HttpClient) {}

  generateToken(byteLength = 32): string {
    const bytes = new Uint8Array(byteLength);
    crypto.getRandomValues(bytes);
    return this.base64UrlEncode(bytes);
  }

  async startLogin(): Promise<{ registrationUrl: string }> {
    const redirectUri = new URL('/oidc/callback', window.location.origin).toString();

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
    const redirectUri = new URL('/local/callback', window.location.origin).toString();

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
    const local = await this.authenticateLocal(token);
    if (local.authenticated) return local;
    return await this.authenticate(token);
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
  }

  async authenticate(token?: string): Promise<AuthenticateResponse> {
    const t = token ?? this.getToken();
    if (!t) return { authenticated: false, user_info: null };

    const url = `${this.apiBase}/api/v1/oidc/authenticate`;
    const headers = new HttpHeaders({ Authorization: `${t}` });

    try {
      return await firstValueFrom(this.http.post<AuthenticateResponse>(url, {}, { headers }));
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
      return await firstValueFrom(this.http.post<AuthenticateResponse>(url, {}, { headers }));
    } catch {
      return { authenticated: false, user_info: null };
    }
  }

  private base64UrlEncode(bytes: Uint8Array): string {
    let binary = '';
    for (const b of bytes) binary += String.fromCharCode(b);

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
