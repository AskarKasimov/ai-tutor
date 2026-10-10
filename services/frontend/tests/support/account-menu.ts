import { fireEvent, screen } from '@testing-library/react'

// Radix menus open from the keyboard in jsdom, which lacks pointer events.
export async function logOut() {
  const trigger = await screen.findByRole('button', { name: /Меню профиля/ })
  fireEvent.keyDown(trigger, { key: 'Enter' })
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Выйти' }))
}
