import { Component, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, Router, RouterModule } from '@angular/router';
import { Auth } from '../../services/auth';

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
    this.message.set('Preparing sign-in…');
    this.registrationUrl.set(null);

    try {
      const { registrationUrl } = await this.auth.startLogin();
      this.registrationUrl.set(registrationUrl);
      this.state.set('idle');
      this.message.set('Continue to registration to complete sign-in.');
    } catch {
      this.state.set('error');
      this.message.set('Could not start login. Please try again.');
    }
  }

  private async validate(): Promise<void> {
    const res = await this.auth.authenticate();
    if (res.authenticated) {
      this.state.set('authenticated');
      this.message.set('You are signed in.');
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

    // TODO
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

    const res = await this.auth.authenticate(token);
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
  }
}
