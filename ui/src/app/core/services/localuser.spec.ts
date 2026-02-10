import { TestBed } from '@angular/core/testing';

import { Localuser } from './localuser';

describe('Localuser', () => {
  let service: Localuser;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(Localuser);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
