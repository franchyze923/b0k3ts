import { TestBed } from '@angular/core/testing';

import { Cephbuckets } from './cephbuckets';

describe('Cephbuckets', () => {
  let service: Cephbuckets;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(Cephbuckets);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
