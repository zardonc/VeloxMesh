# Dashboard README Design

**Date:** 2026-07-17

## Goal

Create a bilingual `dashboard/README.md` that lets a reviewer understand, run, test, and evaluate the VeloxMesh Dashboard without exposing local secrets.

## Audience

- Capstone instructors and reviewers
- VeloxMesh gateway collaborators
- Developers running the Dashboard locally

## Structure

The README will include overview, architecture, implemented Admin and Customer features, repository layout, prerequisites, local setup, environment variable names, test commands, security guidance, limitations, and links to the parent VeloxMesh project.

## Content Rules

- English is the primary GitHub-facing text with concise Chinese explanations.
- Commands must match the current Go, npm, Vite, and Playwright setup.
- No real API key, password, verification code, or local secret may appear.
- Implemented and placeholder Admin pages must be distinguished explicitly.
- Generated artifacts and `.env2.local` must be documented as local-only files.

