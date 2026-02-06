import { TestBed } from '@angular/core/testing';

import { OidcConfig } from './oidc-config';

describe('OidcConfig', () => {
  let service: OidcConfig;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(OidcConfig);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
