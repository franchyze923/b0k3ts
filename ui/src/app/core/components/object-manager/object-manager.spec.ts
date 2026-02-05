import { ComponentFixture, TestBed } from '@angular/core/testing';

import { ObjectManager } from './object-manager';

describe('ObjectManager', () => {
  let component: ObjectManager;
  let fixture: ComponentFixture<ObjectManager>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [ObjectManager]
    })
    .compileComponents();

    fixture = TestBed.createComponent(ObjectManager);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
