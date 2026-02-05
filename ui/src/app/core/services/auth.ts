import { Injectable } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { firstValueFrom } from 'rxjs';

type AuthenticateResponse = {
  authenticated: boolean;
  // You can extend this with user/profile fields if your backend returns them.
  // user?: { id: string; email?: string; name?: string };
};

type StartLoginResponse = {
  registrationUrl?: string;
  registration_url?: string;
  url?: string;
};

@Injectable({
  providedIn: 'root',
})
export class Auth {
  private readonly apiBase = ''; // keep '' for same-origin; set e.g. 'https://api.example.com' if needed

  private readonly storageTokenKey = 'oidc.token';
  private readonly storageStateKey = 'oidc.state';

  constructor(private readonly http: HttpClient) {}

  /**
   * Generates a cryptographically-strong token (base64url) for OIDC `state` (and/or `nonce`).
   */
  generateToken(byteLength = 32): string {
    const bytes = new Uint8Array(byteLength);
    crypto.getRandomValues(bytes);
    return this.base64UrlEncode(bytes);
  }

  /**
   * Starts login by requesting the backend OIDC login endpoint.
   * Backend returns JSON with a registration URL which should be displayed to the user as a link.
   */
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



  /**
   * Accepts token from redirect callback and stores it (session-scoped by default).
   */
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

  /**
   * Verifies that the callback `state` matches what we issued before redirecting.
   */
  verifyCallbackState(receivedState: string | null): boolean {
    const expected = sessionStorage.getItem(this.storageStateKey);
    return !!receivedState && !!expected && receivedState === expected;
  }

  /**
   * Validates auth by calling backend `/api/v1/oidc/authenticate`.
   * Assumes backend accepts Authorization: Bearer <token>.
   * If your backend expects JSON { token }, tell me and I’ll adjust.
   */
  async authenticate(token?: string): Promise<AuthenticateResponse> {
    const t = token ?? this.getToken();
    if (!t) return { authenticated: false };

    const url = `${this.apiBase}/api/v1/oidc/authenticate`;
    const headers = new HttpHeaders({ Authorization: `${t}` });

    try {
      return await firstValueFrom(this.http.post<AuthenticateResponse>(url, {}, { headers }));
    } catch {
      return { authenticated: false };
    }
  }

  private base64UrlEncode(bytes: Uint8Array): string {
    let binary = '';
    for (const b of bytes) binary += String.fromCharCode(b);

    const base64 = btoa(binary);
    return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
  }
}
