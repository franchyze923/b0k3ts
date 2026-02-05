import { TestBed } from '@angular/core/testing';

import { ObjectStorage } from './object-storage';

describe('ObjectStorage', () => {
  let service: ObjectStorage;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(ObjectStorage);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
