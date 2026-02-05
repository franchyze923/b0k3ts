import { Component, signal } from '@angular/core';
import { NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs/operators';
import { ThemeService } from './core/services/theme';
import { Auth } from './core/services/auth';

import { MatSidenavModule } from '@angular/material/sidenav';
import { MatToolbarModule } from '@angular/material/toolbar';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatListModule } from '@angular/material/list';
import {GlobalService} from './core/services/global';
import {AsyncPipe} from '@angular/common';

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    MatSidenavModule,
    MatToolbarModule,
    MatButtonModule,
    MatIconModule,
    MatListModule,
    AsyncPipe
  ],
  templateUrl: './app.html',
  styleUrl: './app.scss',
})
export class App {
  protected readonly title = signal('b0k3ts');
  protected readonly authenticated = signal<boolean>(false);

  constructor(
    protected readonly theme: ThemeService,
    private readonly auth: Auth,
    private readonly router: Router,
    public globalService: GlobalService
  ) {
    // Ensure the theme attribute is applied on app start
    this.theme.apply(this.theme.theme());

    // Initial auth check + re-check after each navigation (e.g. after OIDC callback cleans URL)
    void this.refreshAuth();

    this.router.events
      .pipe(filter((e): e is NavigationEnd => e instanceof NavigationEnd))
      .subscribe(() => {
        void this.refreshAuth();
      });
  }

  toggleTheme(): void {
    this.theme.toggle();
  }

  private async refreshAuth(): Promise<void> {
    const token = this.auth.getToken();
    if (!token) {
      this.authenticated.set(false);
      return;
    }

    const res = await this.auth.authenticate(token);
    if (res.authenticated) {
      this.authenticated.set(true);
      return;
    }

    this.auth.clearToken();
    this.authenticated.set(false);
  }
}
