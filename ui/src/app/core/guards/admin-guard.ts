import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { Auth } from '../services/auth';
import { OidcConfigService } from '../services/oidc-config';

function isAdminUser(userInfo: any, adminGroup?: string): boolean {
  const administrator = userInfo?.administrator === true;

  // Common shapes: groups: string[] OR groups: { name: string }[] OR roles: string[]
  const groups: unknown[] = Array.isArray(userInfo?.groups) ? userInfo.groups : [];
  const roles: unknown[] = Array.isArray(userInfo?.roles) ? userInfo.roles : [];

  const target = String(adminGroup ?? '').trim() || '/Admins';

  const inAdminsGroup =
    groups.some((g) => (typeof g === 'string' ? g : (g as any)?.name) === target) ||
    roles.some((r) => r === target || (target === '/Admins' && r === 'Admins'));

  return administrator || inAdminsGroup;
}

export const adminGuard: CanActivateFn = async (_route, state) => {
  const auth = inject(Auth);
  const router = inject(Router);
  const oidc = inject(OidcConfigService);

  const token = auth.getToken();
  if (!token) {
    return router.createUrlTree(['/login'], {
      queryParams: { returnUrl: state.url },
    });
  }

  const res = await auth.authenticateAny(token);

  if (!res?.authenticated) {
    auth.clearToken();
    return router.createUrlTree(['/login'], {
      queryParams: { returnUrl: state.url },
    });
  }

  let adminGroup: string | undefined;
  try {
    const cfg = await oidc.getConfig();
    adminGroup = cfg.adminGroup;
  } catch {
    // If config can't be loaded, fall back to default '/Admins'
  }

  if (isAdminUser(res.user_info, adminGroup)) {
    return true;
  }

  return router.createUrlTree(['/settings']);
};
