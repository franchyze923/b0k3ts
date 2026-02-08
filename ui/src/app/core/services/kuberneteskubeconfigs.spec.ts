import { TestBed } from '@angular/core/testing';

import { Kuberneteskubeconfigs } from './kuberneteskubeconfigs';

describe('Kuberneteskubeconfigs', () => {
  let service: Kuberneteskubeconfigs;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(Kuberneteskubeconfigs);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});
