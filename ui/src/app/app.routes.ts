import { Routes } from '@angular/router';
import { Login } from './core/components/login/login';
import { ObjectManager } from './core/components/object-manager/object-manager';
import { authGuard } from './core/guards/auth-guard';
import { Settings } from './core/components/settings/settings';
import { BucketConnectionsSettings } from './core/components/settings/bucket-connections-settings/bucket-connections-settings';
import { OidcSettings } from './core/components/settings/oidc-settings/oidc-settings';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'object-manager' },
  { path: 'login', component: Login },
  { path: 'oidc/callback', component: Login },
  { path: 'local/callback', component: Login },

  { path: 'object-manager', component: ObjectManager, canActivate: [authGuard] },

  {
    path: 'settings',
    component: Settings,
    canActivate: [authGuard],
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'buckets' },
      { path: 'buckets', component: BucketConnectionsSettings },
      { path: 'oidc', component: OidcSettings },
    ],
  },

  { path: '**', redirectTo: 'object-manager' },
];
