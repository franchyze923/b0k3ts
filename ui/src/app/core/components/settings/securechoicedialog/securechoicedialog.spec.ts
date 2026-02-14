import { ComponentFixture, TestBed } from '@angular/core/testing';

import { Securechoicedialog } from './securechoicedialog';

describe('Securechoicedialog', () => {
  let component: Securechoicedialog;
  let fixture: ComponentFixture<Securechoicedialog>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Securechoicedialog]
    })
    .compileComponents();

    fixture = TestBed.createComponent(Securechoicedialog);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});
