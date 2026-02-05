import { Routes } from '@angular/router';
import { Login } from './core/components/login/login';
import { ObjectManager } from './core/components/object-manager/object-manager';
import { authGuard } from './core/guards/auth-guard';

export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'object-manager' },
  { path: 'login', component: Login },
  { path: 'oidc/callback', component: Login },

  { path: 'object-manager', component: ObjectManager, canActivate: [authGuard] },

  { path: '**', redirectTo: 'object-manager' },
];
