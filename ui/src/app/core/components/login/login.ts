import { Component, computed, signal } from '@angular/core';
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
  protected readonly tokenPresent = computed(() => !!this.auth.getToken());

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
    this.message.set('Opening secure sign-in…');
    this.auth.startLogin();
  }

  async signOut(): Promise<void> {
    this.auth.clearToken();
    this.state.set('idle');
    this.message.set('Signed out. See you soon.');
    await this.router.navigateByUrl('/login');
  }

  async validate(): Promise<void> {
    const res = await this.auth.authenticate();
    if (res.authenticated) {
      this.state.set('authenticated');
      this.message.set('Authenticated. Token validated by server.');
    } else {
      this.state.set('error');
      this.message.set('Token not accepted by server. Please sign in again.');
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

    if (error) {
      this.state.set('error');
      this.message.set(`Authentication error: ${error}`);
      this.auth.clearToken();
      return;
    }

    if (!this.auth.verifyCallbackState(state)) {
      this.state.set('error');
      this.message.set('Security check failed (state mismatch). Please try again.');
      this.auth.clearToken();
      return;
    }

    this.auth.setToken(token);
    this.message.set('Token received. Verifying with server…');

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
    this.message.set('Welcome back. You are signed in.');
  }


}
