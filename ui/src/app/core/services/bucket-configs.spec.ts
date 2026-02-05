import { TestBed } from '@angular/core/testing';

import { BucketConfigs } from './bucket-configs';

describe('BucketConfigs', () => {
  let service: BucketConfigs;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(BucketConfigs);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
