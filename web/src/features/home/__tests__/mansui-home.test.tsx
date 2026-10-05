/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { MansuiHome, ModelMarquee } from '../components/mansui-home'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: { model?: string }) =>
      values?.model ? key.replace('{{model}}', values.model) : key,
  }),
}))

test('renders pricing and canvas destinations as native external links', () => {
  render(<MansuiHome isAuthenticated={false} />)

  const destinations = [
    {
      name: 'Explore API and pricing',
      href: 'https://api.lolicon.beer/pricing',
      count: 2,
    },
    {
      name: 'Open creative canvas',
      href: 'https://love.lolicon.beer',
      count: 3,
    },
  ]

  for (const destination of destinations) {
    const links = screen.getAllByRole('link', { name: destination.name })
    expect(links).toHaveLength(destination.count)
    for (const link of links) {
      expect(link.tagName).toBe('A')
      expect(link).not.toHaveAttribute('role', 'button')
      expect(link).toHaveAttribute('href', destination.href)
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    }
  }
})

test('keeps the marquee paused when pointer leaves while focus remains inside', () => {
  render(<ModelMarquee />)
  const marquee = screen.getByTestId('model-marquee')
  const firstModel = screen.getByRole('button', {
    name: 'DeepSeek Chat deepseek-chat model information',
  })

  expect(marquee).toHaveAttribute('data-paused', 'false')
  fireEvent.pointerEnter(marquee)
  expect(marquee).toHaveAttribute('data-paused', 'true')

  firstModel.focus()
  expect(firstModel).toHaveFocus()
  fireEvent.pointerLeave(marquee)
  expect(marquee).toHaveAttribute('data-paused', 'true')

  fireEvent.blur(firstModel, { relatedTarget: document.body })
  expect(marquee).toHaveAttribute('data-paused', 'false')
})

test('supports persistent manual pause and resume controls', async () => {
  const user = userEvent.setup()
  render(<ModelMarquee />)
  const marquee = screen.getByTestId('model-marquee')
  const pauseButton = screen.getByRole('button', { name: 'Pause' })

  expect(pauseButton).toHaveAttribute('aria-pressed', 'false')
  await user.click(pauseButton)

  const resumeButton = screen.getByRole('button', { name: 'Resume' })
  expect(resumeButton).toHaveAttribute('aria-pressed', 'true')
  expect(marquee).toHaveAttribute('data-paused', 'true')

  await user.click(resumeButton)
  expect(screen.getByRole('button', { name: 'Pause' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
  expect(marquee).toHaveAttribute('data-paused', 'false')
})

test('shows model capability details when a badge receives keyboard focus', async () => {
  render(<ModelMarquee />)
  const modelButton = screen.getByRole('button', {
    name: 'DeepSeek Chat deepseek-chat model information',
  })

  modelButton.focus()
  expect(modelButton).toHaveFocus()

  await waitFor(() => {
    expect(screen.getByRole('tooltip')).toBeVisible()
  })
  expect(
    within(screen.getByRole('tooltip')).getByText(
      'Chat and text generation for general-purpose AI workflows.'
    )
  ).toBeVisible()
})

test('shows duplicate model details on hover without duplicating navigation or IDs', async () => {
  const user = userEvent.setup()
  render(<ModelMarquee />)

  const marquee = screen.getByTestId('model-marquee')
  const duplicateGroup = marquee.querySelector<HTMLElement>(
    ".mansui-marquee-group[aria-hidden='true']"
  )
  expect(duplicateGroup).not.toBeNull()

  const duplicateTriggers = duplicateGroup?.querySelectorAll<HTMLElement>(
    '.mansui-model-trigger'
  )
  expect(duplicateTriggers).toHaveLength(5)
  expect(
    duplicateGroup?.querySelectorAll(
      'button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
    )
  ).toHaveLength(0)
  expect(
    document.querySelectorAll('#mansui-model-fact-deepseek-chat')
  ).toHaveLength(1)

  const deepSeekDuplicate = within(duplicateGroup as HTMLElement)
    .getByText('DeepSeek Chat')
    .closest<HTMLElement>('.mansui-model-trigger')
  expect(deepSeekDuplicate).not.toBeNull()

  await user.hover(deepSeekDuplicate as HTMLElement)

  await waitFor(() => {
    expect(screen.getByRole('tooltip', { hidden: true })).toBeVisible()
  })
  expect(
    within(screen.getByRole('tooltip', { hidden: true })).getByText(
      'Chat and text generation for general-purpose AI workflows.'
    )
  ).toBeVisible()
})

test('exposes model IDs and facts once while hiding the visual duplicate loop', () => {
  render(<ModelMarquee />)

  const models = [
    {
      name: 'DeepSeek Chat',
      id: 'deepseek-chat',
      fact: 'Chat and text generation for general-purpose AI workflows.',
    },
    {
      name: 'GPT-4.1',
      id: 'gpt-4.1',
      fact: 'Text and code generation for instruction-driven workflows.',
    },
    {
      name: 'MiniMax H3',
      id: 'MiniMax-H3',
      fact: 'Create videos from text, images, and multimodal references.',
    },
    {
      name: 'Seedream 4.0',
      id: 'doubao-seedream-4-0-250828',
      fact: 'Create and edit images through the image generation API.',
    },
    {
      name: 'Seedance 2.5',
      id: 'doubao-seedance-2-5-260628',
      fact: 'Create videos from text, images, and video references.',
    },
  ]

  const marquee = screen.getByTestId('model-marquee')
  const primaryGroup = marquee.querySelector(
    '.mansui-marquee-group:not([aria-hidden])'
  )
  const duplicateGroup = marquee.querySelector(
    ".mansui-marquee-group[aria-hidden='true']"
  )
  expect(primaryGroup).not.toBeNull()
  expect(duplicateGroup).toHaveAttribute('aria-hidden', 'true')

  for (const model of models) {
    const modelButton = screen.getByRole('button', {
      name: `${model.name} ${model.id} model information`,
    })
    expect(modelButton).toHaveAccessibleDescription(model.fact)
    expect(
      within(primaryGroup as HTMLElement).getByText(model.id)
    ).toBeVisible()
  }

  expect(
    screen.getAllByRole('button', { name: / model information$/ })
  ).toHaveLength(models.length)
  expect(
    within(duplicateGroup as HTMLElement).queryByRole('button')
  ).not.toBeInTheDocument()
})
