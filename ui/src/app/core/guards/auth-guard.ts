import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import {Auth} from '../services/auth';


export const authGuard: CanActivateFn = async (route, state) => {
  const auth = inject(Auth);
  const router = inject(Router);

  const token = auth.getToken();
  if (!token) {
    return router.createUrlTree(['/login'], {
      queryParams: { returnUrl: state.url },
    });
  }

  const res = await auth.authenticate(token);
  if (res.authenticated) return true;

  auth.clearToken();
  return router.createUrlTree(['/login'], {
    queryParams: { returnUrl: state.url },
  });
};
