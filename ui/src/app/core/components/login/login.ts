import { Component, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, Router, RouterModule } from '@angular/router';
import { Auth, AuthenticateResponse } from '../../services/auth';

type UiState = 'idle' | 'redirecting' | 'processing-callback' | 'authenticated' | 'error';

@Component({
  selector: 'app-login',
  imports: [CommonModule, RouterModule],
  templateUrl: './login.html',
  styleUrl: './login.scss',
})
export class Login {

  protected readonly state = signal<UiState>('idle');
  protected readonly message = signal<string>('Sign in with your organization account.');
  protected readonly registrationUrl = signal<string | null>(null);

  protected readonly localUsername = signal<string>('');
  protected readonly localPassword = signal<string>('');

  constructor(
    private readonly auth: Auth,
    private readonly route: ActivatedRoute,
    private readonly router: Router,
  ) {
    this.handleCallbackIfPresent().catch(() => {
      this.state.set('error');
      this.message.set('Login failed. Please try again.');
    });
  }
  async signIn(): Promise<void> {
    this.state.set('redirecting');
    this.message.set('Redirecting…');
    this.registrationUrl.set(null);

    try {
      const { registrationUrl } = await this.auth.startLogin();

      // Automatically go to registration (no button shown to the user)
      window.location.assign(registrationUrl);
    } catch {
      this.state.set('error');
      this.message.set('Could not start login. Please try again.');
    }
  }

  async signInLocal(): Promise<void> {
    this.state.set('redirecting');
    this.message.set('Signing in…');
    this.registrationUrl.set(null);

    try {
      const { registrationUrl, token } = await this.auth.startLocalLogin({
        username: this.localUsername().trim(),
        password: this.localPassword(),
      });

      // Clear password as early as possible
      this.localPassword.set('');

      // Direct-token flow (no redirect needed)
      if (token) {
        this.auth.setToken(token);
        this.message.set('Finishing sign-in…');

        const res = await this.auth.authenticateLocal(token);
        if (!res.authenticated) {
          this.state.set('error');
          this.message.set('Server rejected the token. Please sign in again.');
          this.auth.clearToken();
          return;
        }

        this.state.set('authenticated');
        this.message.set('You are signed in.');
        await this.router.navigateByUrl('/dashboard');

        return;
      }

      // Redirect flow (OIDC-like)
      if (registrationUrl) {
        window.location.assign(registrationUrl);
        return;
      }

      // Should not happen due to checks in service
      throw new Error('No token or redirect URL returned.');
    } catch {
      this.state.set('error');
      this.message.set('Could not start local login. Please try again.');
      this.localPassword.set('');
    }
  }

  private async validate(): Promise<void> {
    const res = await this.auth.authenticateAny();
    if (res.authenticated) {
      this.state.set('authenticated');
      this.message.set('You are signed in.');
      await this.router.navigateByUrl('/dashboard');

    } else {
      this.state.set('error');
      this.message.set('Session not accepted. Please sign in again.');
      this.auth.clearToken();
    }
  }

  private async handleCallbackIfPresent(): Promise<void> {
    const qp = this.route.snapshot.queryParamMap;
    const token = qp.get('token');
    const state = qp.get('state');
    const error = qp.get('error');

    if (!token && !error) {
      // Not a callback; optionally auto-validate if a token exists.
      if (this.auth.getToken()) {
        this.state.set('processing-callback');
        this.message.set('Checking session…');
        await this.validate();
      }
      return;
    }

    this.state.set('processing-callback');
    this.registrationUrl.set(null);

    if (error) {
      this.state.set('error');
      this.message.set(`Authentication error: ${error}`);
      this.auth.clearToken();
      return;
    }

    // if (!this.auth.verifyCallbackState(state)) {
    //   this.state.set('error');
    //   this.message.set('Security check failed. Please try again.');
    //   this.auth.clearToken();
    //   return;
    // }

    if (!token) {
      this.state.set('error');
      this.message.set('Token not found. Please try again.');
      this.auth.clearToken();
      return;
    }

    this.auth.setToken(token);
    this.message.set('Finishing sign-in…');

    const callbackPath = this.route.snapshot.routeConfig?.path ?? '';
    const isLocalCallback = callbackPath === 'local/callback';

    const res: AuthenticateResponse = isLocalCallback
      ? await this.auth.authenticateLocal(token)
      : await this.auth.authenticateAny(token);

    if (!res.authenticated) {
      this.state.set('error');
      this.message.set('Server rejected the token. Please sign in again.');
      this.auth.clearToken();
      return;
    }

    // Clean up URL (remove token from the address bar / history)
    await this.router.navigate([], { queryParams: {}, replaceUrl: true });

    this.state.set('authenticated');
    this.message.set('You are signed in.');
    await this.router.navigateByUrl('/dashboard');

  }
}
