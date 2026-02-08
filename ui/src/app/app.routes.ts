import { Routes } from '@angular/router';
import { Login } from './core/components/login/login';
import { ObjectManager } from './core/components/object-manager/object-manager';
import { authGuard } from './core/guards/auth-guard';
import { Settings } from './core/components/settings/settings';
import { BucketConnectionsSettings } from './core/components/settings/bucket-connections-settings/bucket-connections-settings';
import { OidcSettings } from './core/components/settings/oidc-settings/oidc-settings';
import { Kubernetes } from './core/components/settings/kubernetes/kubernetes';
import { adminGuard } from './core/guards/admin-guard';
import { Plugins } from './plugins/components/plugins/plugins';
import { Ceph } from './plugins/components/ceph/ceph';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'object-manager' },
  { path: 'login', component: Login },
  { path: 'oidc/callback', component: Login },
  { path: 'local/callback', component: Login },

  { path: 'object-manager', component: ObjectManager, canActivate: [authGuard] },

  {
    path: 'settings',
    component: Settings,

    children: [
      { path: '', pathMatch: 'full', redirectTo: 'buckets' },
      { path: 'buckets', component: BucketConnectionsSettings, canActivate: [authGuard] },
      { path: 'oidc', component: OidcSettings, canActivate: [adminGuard] },
      { path: 'kubernetes', component: Kubernetes, canActivate: [adminGuard] },
    ],
  },

  {
    path: 'plugins',
    component: Plugins,
    canActivate: [adminGuard],
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'ceph' },
      { path: 'ceph', component: Ceph, canActivate: [adminGuard] },
    ],
  },

  { path: '**', redirectTo: 'object-manager' },
];
