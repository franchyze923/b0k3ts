import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { Auth } from '../services/auth';

function isAdminUser(userInfo: any): boolean {
  const administrator = userInfo?.administrator === true;

  // Common shapes: groups: string[] OR groups: { name: string }[] OR roles: string[]
  const groups: unknown[] = Array.isArray(userInfo?.groups) ? userInfo.groups : [];
  const roles: unknown[] = Array.isArray(userInfo?.roles) ? userInfo.roles : [];

  const inAdminsGroup =
    groups.some((g) => (typeof g === 'string' ? g : (g as any)?.name) === '/Admins') ||
    roles.some((r) => r === 'Admins');

  return administrator || inAdminsGroup;
}

export const adminGuard: CanActivateFn = async (_route, state) => {
  const auth = inject(Auth);
  const router = inject(Router);

  const token = auth.getToken();
  if (!token) {
    return router.createUrlTree(['/login'], {
      queryParams: { returnUrl: state.url },
    });
  }

  // Reuse your existing server-side validation to get user_info
  const res = await auth.authenticateAny(token);

  if (!res?.authenticated) {
    auth.clearToken();
    return router.createUrlTree(['/login'], {
      queryParams: { returnUrl: state.url },
    });
  }

  if (isAdminUser(res.user_info)) {
    return true;
  }

  // Not authorized: redirect somewhere safe
  return router.createUrlTree(['/settings']);
};
