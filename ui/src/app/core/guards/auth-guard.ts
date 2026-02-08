import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { Auth } from '../services/auth';
import { GlobalService } from '../services/global';

export const authGuard: CanActivateFn = async (route, state) => {
  const globalService = inject(GlobalService);

  const auth = inject(Auth);
  const router = inject(Router);

  const token = auth.getToken();
  if (!token) {
    return router.createUrlTree(['/login'], {
      queryParams: { returnUrl: state.url },
    });
  }

  const res = await auth.authenticateAny(token);
  if (res.authenticated) {
    const email = res.user_info?.email || 'Unknown User';
    globalService.updateTitle('Welcome ' + email);
    return true;
  }

  auth.clearToken();
  return router.createUrlTree(['/login'], {
    queryParams: { returnUrl: state.url },
  });
};
