import { Routes } from '@angular/router';
import { Login } from './core/components/login/login';


export const routes: Routes = [
  { path: '', pathMatch: 'full', redirectTo: 'login' },
  { path: 'login', component: Login },
  { path: 'oidc/callback', component: Login },
  { path: '**', redirectTo: 'login' },
];
