<script lang="ts">
  import { onMount } from 'svelte'
  import { ArrowUp, Moon, Paperclip, Plus, Square, Sun, Trash2, X } from '@lucide/svelte'
  import { CancelGeneration, ChooseImages, CreateConversation, DeleteConversation, GetConversation, GetModelStatus, GetUserAvatar, ListConversations, SendMessage } from '../wailsjs/go/main/App.js'
  import { BrowserOpenURL, EventsOn, Quit, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime/runtime.js'
  import fmChatIcon from './assets/images/fm-chat.png'
  import { renderMarkdown } from './lib/markdown'

  type Attachment = { id: string; name: string; mimeType: string }
  type Message = { id: string; role: string; content: string; createdAt: string; attachments: Attachment[] }
  type Conversation = { id: string; title: string; createdAt: string; updatedAt: string; messages: Message[] }
  type ModelStatus = { available: boolean; message: string }
  type ChatEvent = { conversationID: string; messageID: string; text: string }

  let conversations = $state<Conversation[]>([])
  let activeConversation = $state<Conversation | null>(null)
  let attachments = $state<Attachment[]>([])
  let draft = $state('')
  let modelStatus = $state<ModelStatus>({ available: false, message: 'Checking model…' })
  let userAvatar = $state('')
  let generatingConversationID = $state<string | null>(null)
  let generatingMessageID = $state<string | null>(null)
  let errorMessage = $state('')
  let conversationToDelete = $state<Conversation | null>(null)
  let theme = $state<'light' | 'dark'>('light')
  let windowFocused = $state(true)
  let composer: HTMLTextAreaElement
  let messagesContainer: HTMLDivElement
  let deleteTrigger: HTMLButtonElement
  let hasManualTheme = false

  const isGenerating = $derived(generatingConversationID === activeConversation?.id)
  const canSend = $derived(modelStatus.available && !isGenerating && (draft.trim().length > 0 || attachments.length > 0))

  onMount(() => {
    windowFocused = document.hasFocus()
    const removeThemeListener = initializeTheme()
    const removeChunkListener = EventsOn('chat:chunk', (event: ChatEvent) => {
      if (event.conversationID !== activeConversation?.id) return
      const message = activeConversation.messages.find((item) => item.id === event.messageID)
      if (message) {
        message.content += event.text
        requestAnimationFrame(() => scrollMessagesToBottom())
      }
    })
    const removeCompleteListener = EventsOn('chat:complete', (event: ChatEvent) => {
      if (event.messageID === generatingMessageID) {
        generatingConversationID = null
        generatingMessageID = null
      }
      requestAnimationFrame(() => scrollMessagesToBottom('smooth'))
      refreshConversations()
    })
    const removeErrorListener = EventsOn('chat:error', (event: ChatEvent) => {
      if (event.messageID === generatingMessageID) {
        generatingConversationID = null
        generatingMessageID = null
        errorMessage = event.text
      }
      requestAnimationFrame(() => scrollMessagesToBottom('smooth'))
      refreshConversations()
    })
    initialize()
    const onMessagesClick = (event: MouseEvent) => { void handleCodeCopy(event) }
    messagesContainer.addEventListener('click', onMessagesClick)

    return () => {
      removeChunkListener()
      removeCompleteListener()
      removeErrorListener()
      removeThemeListener()
      messagesContainer.removeEventListener('click', onMessagesClick)
    }
  })

  function initializeTheme() {
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const storedTheme = localStorage.getItem('fm-chat-theme')
    if (storedTheme === 'dark' || storedTheme === 'light') {
      hasManualTheme = true
      applyTheme(storedTheme)
    } else {
      applyTheme(media.matches ? 'dark' : 'light')
    }

    const followSystemTheme = (event: MediaQueryListEvent) => {
      if (!hasManualTheme) applyTheme(event.matches ? 'dark' : 'light')
    }
    media.addEventListener('change', followSystemTheme)
    return () => media.removeEventListener('change', followSystemTheme)
  }

  function applyTheme(nextTheme: 'light' | 'dark') {
    theme = nextTheme
    document.documentElement.dataset.theme = nextTheme
  }

  function toggleTheme() {
    hasManualTheme = true
    const nextTheme = theme === 'dark' ? 'light' : 'dark'
    localStorage.setItem('fm-chat-theme', nextTheme)
    applyTheme(nextTheme)
  }

  async function initialize() {
    try {
      try { userAvatar = await GetUserAvatar() } catch { userAvatar = '' }
      modelStatus = await GetModelStatus()
      await refreshConversations()
      if (conversations.length > 0) await selectConversation(conversations[0].id)
    } catch (error) {
      errorMessage = errorText(error)
    }
  }

  async function refreshConversations() { conversations = await ListConversations() }

  async function selectConversation(id: string) {
    try {
      activeConversation = await GetConversation(id)
      attachments = []
      draft = ''
      requestAnimationFrame(resizeComposer)
      errorMessage = ''
    } catch (error) { errorMessage = errorText(error) }
  }

  async function createConversation() {
    try {
      const conversation = await CreateConversation()
      conversations = [conversation, ...conversations]
      activeConversation = conversation
      attachments = []
      draft = ''
      requestAnimationFrame(resizeComposer)
      errorMessage = ''
      composer?.focus()
    } catch (error) { errorMessage = errorText(error) }
  }

  async function confirmDeletion() {
    if (!conversationToDelete) return
    const deletedConversationID = conversationToDelete.id
    try {
      await DeleteConversation(deletedConversationID)
      dismissDeletion()
      await refreshConversations()
      if (activeConversation?.id === deletedConversationID) {
        activeConversation = null
        if (conversations[0]) await selectConversation(conversations[0].id)
      }
    } catch (error) {
      dismissDeletion()
      errorMessage = errorText(error)
    }
  }

  function requestDeletion(conversation: Conversation, event: MouseEvent) {
    deleteTrigger = event.currentTarget as HTMLButtonElement
    conversationToDelete = conversation
  }

  function dismissDeletion() {
    conversationToDelete = null
    requestAnimationFrame(() => deleteTrigger?.focus())
  }

  async function chooseImages() {
    try { attachments = [...attachments, ...(await ChooseImages())] }
    catch (error) { errorMessage = errorText(error) }
  }

  function removeAttachment(id: string) { attachments = attachments.filter((attachment) => attachment.id !== id) }

  function resizeComposer() {
    if (!composer) return
    composer.style.height = 'auto'
    const maxHeight = Number.parseFloat(getComputedStyle(composer).maxHeight)
    const nextHeight = Math.min(composer.scrollHeight, maxHeight)
    composer.style.height = `${nextHeight}px`
    composer.style.overflowY = composer.scrollHeight > maxHeight ? 'auto' : 'hidden'
  }

  function useSuggestion(value: string) {
    draft = value
    requestAnimationFrame(resizeComposer)
    composer?.focus()
  }

  function scrollMessagesToBottom(behavior: ScrollBehavior = 'auto') {
    if (!messagesContainer) return
    messagesContainer.scrollTo({ top: messagesContainer.scrollHeight, behavior })
  }

  async function sendMessage() {
    if (!canSend) return

    const prompt = draft.trim()
    const pendingAttachments = attachments.map((attachment) => ({
      id: attachment.id,
      name: attachment.name,
      mimeType: attachment.mimeType,
    }))

    if (!activeConversation) await createConversation()
    if (!activeConversation) return

    const conversationID = activeConversation.id
    try {
      errorMessage = ''
      const assistantMessage = await SendMessage(conversationID, prompt, pendingAttachments)
      draft = ''
      attachments = []
      requestAnimationFrame(resizeComposer)
      generatingConversationID = conversationID
      generatingMessageID = assistantMessage.id
      activeConversation = await GetConversation(conversationID)
      requestAnimationFrame(() => scrollMessagesToBottom('smooth'))
      requestAnimationFrame(resizeComposer)
      await refreshConversations()
    } catch (error) {
      generatingConversationID = null
      generatingMessageID = null
      errorMessage = errorText(error)
    }
  }

  function cancelGeneration() { if (activeConversation) CancelGeneration(activeConversation.id) }
  async function handleCodeCopy(event: MouseEvent) {
    const target = event.target as HTMLElement
    const button = target.closest<HTMLButtonElement>('[data-copy-code]')
    if (!button) return

    const code = button.closest('pre')?.querySelector('code')?.textContent ?? ''
    if (!code) return

    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(code)
      } else {
        const textarea = document.createElement('textarea')
        textarea.value = code
        textarea.style.position = 'fixed'
        textarea.style.opacity = '0'
        document.body.appendChild(textarea)
        textarea.select()
        document.execCommand('copy')
        textarea.remove()
      }
      button.classList.add('copied')
      button.setAttribute('aria-label', 'Code copied')
      button.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg><span>Copied</span>'
      window.setTimeout(() => {
        if (!button.isConnected) return
        button.classList.remove('copied')
        button.setAttribute('aria-label', 'Copy code')
        button.innerHTML = '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>'
      }, 1500)
    } catch {
      button.setAttribute('aria-label', 'Copy failed')
    }
  }
  function handleComposerKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' && event.metaKey) { event.preventDefault(); sendMessage() }
  }
  function openFmgo(event: MouseEvent) {
    event.preventDefault()
    BrowserOpenURL('https://github.com/ruanklein/fmgo')
  }
  function errorText(error: unknown) { return error instanceof Error ? error.message : String(error) }
  function relativeDate(value: string) {
    const date = new Date(value)
    if (date.toDateString() === new Date().toDateString()) return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    return date.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }
</script>

<svelte:head><title>FM Chat</title></svelte:head>

<svelte:window
  onkeydown={(event) => event.key === 'Escape' && conversationToDelete && dismissDeletion()}
  onfocus={() => (windowFocused = true)}
  onblur={() => (windowFocused = false)}
/>

<div class="window-shell">
  <header class={`window-titlebar${windowFocused ? '' : ' window-inactive'}`} style="--wails-draggable: drag">
    <div class="window-titlebar-sidebar">
      <div class="window-controls">
        <button class="window-control close-control" style="--wails-draggable: no-drag" aria-label="Close FM Chat" onclick={() => Quit()}>
          <svg class="traffic-light-glyph" viewBox="0 0 12 12" aria-hidden="true"><path d="m4 4 4 4m0-4L4 8" /></svg>
        </button>
        <button class="window-control minimize-control" style="--wails-draggable: no-drag" aria-label="Minimize FM Chat" onclick={() => WindowMinimise()}>
          <svg class="traffic-light-glyph" viewBox="0 0 12 12" aria-hidden="true"><path d="M3.25 6h5.5" /></svg>
        </button>
        <button class="window-control maximize-control" style="--wails-draggable: no-drag" aria-label="Maximize or restore FM Chat" onclick={() => WindowToggleMaximise()}>
          <svg class="traffic-light-glyph" viewBox="0 0 12 12" aria-hidden="true"><path d="M4.25 3.25h4.5v4.5M7.75 3.25l-4.5 4.5M7.75 8.75h-4.5v-4.5" /></svg>
        </button>
      </div>
      <button class="new-chat-titlebar" style="--wails-draggable: no-drag" aria-label="New chat" onclick={createConversation}><Plus size={15} strokeWidth={2.1} /></button>
    </div>
    <div class="window-titlebar-content">
      <span class="window-chat-title">{activeConversation?.title ?? 'New chat'}</span>
      <button class="theme-toggle titlebar-theme-toggle" style="--wails-draggable: no-drag" aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`} onclick={toggleTheme}>
        {#if theme === 'dark'}<Sun size={16} strokeWidth={1.8} />{:else}<Moon size={16} strokeWidth={1.8} />{/if}
      </button>
    </div>
  </header>

  <main class="app-shell">
    <aside class="sidebar">
    <div class="history-heading">CONVERSATIONS</div>
    <nav class="history" aria-label="Conversation history">
      {#if conversations.length === 0}<p class="empty-history">Your chats will appear here.</p>{/if}
      {#each conversations as conversation (conversation.id)}
        <div class:active={conversation.id === activeConversation?.id} class="conversation-row">
          <button class="conversation" onclick={() => selectConversation(conversation.id)}><span class="conversation-title">{conversation.title}</span><span class="conversation-date">{relativeDate(conversation.updatedAt)}</span></button>
          <button class="delete-chat" aria-label={`Delete ${conversation.title}`} disabled={conversation.id === generatingConversationID} onclick={(event) => requestDeletion(conversation, event)}><X size={14} strokeWidth={2} /></button>
        </div>
      {/each}
    </nav>
    <a class="model-status fmgo-sidebar-credit" href="https://github.com/ruanklein/fmgo" onclick={openFmgo}><span class="fmgo-credit-label">Built with fmgo</span></a>
    </aside>

    <section class="chat-panel">
    {#if errorMessage}<div class="error-banner" role="alert"><div><strong>Unable to complete that request.</strong><span>{errorMessage}</span></div><button aria-label="Dismiss error" onclick={() => (errorMessage = '')}><X size={16} /></button></div>{/if}
    <div class="messages" bind:this={messagesContainer}>
      {#if !activeConversation || activeConversation.messages.length === 0}
        <section class="welcome"><img class="welcome-icon" src={fmChatIcon} alt="" /><p class="eyebrow">FOUNDATION MODELS</p><h3>What would you like to explore?</h3><p>Ask a question, work through an idea, or attach an image. Your conversation stays on this Mac.</p><div class="suggestions"><button onclick={() => useSuggestion('Explain how Foundation Models work in one paragraph.')}>Explain Foundation Models</button><button onclick={() => useSuggestion('Help me write a concise email.')}>Draft an email</button><button onclick={() => useSuggestion('What can you tell me about this image?')}>Analyze an image</button></div></section>
      {:else}
        <div class="message-list">
          {#each activeConversation.messages as message (message.id)}
            <article class:assistant={message.role === 'assistant'} class="message"><div class="avatar">{#if message.role === 'assistant'}<img class:thinking={message.id === generatingMessageID} src={fmChatIcon} alt="FM Chat" />{:else if userAvatar}<img src={userAvatar} alt="You" />{:else}<span>You</span>{/if}</div><div class="message-body"><div class="message-meta"><strong>{message.role === 'assistant' ? 'FM Chat' : 'You'}</strong><span>{relativeDate(message.createdAt)}</span></div>{#if message.attachments.length > 0}<div class="attachment-summary">{#each message.attachments as attachment (attachment.id)}<span>▧ {attachment.name}</span>{/each}</div>{/if}{#if message.content}{#if message.role === 'assistant'}<div class="message-content">{@html renderMarkdown(message.content)}</div>{:else}<p class="message-content">{message.content}</p>{/if}{:else if message.role === 'assistant' && message.id === generatingMessageID}<span class="typing"><i></i><i></i><i></i></span>{/if}</div></article>
          {/each}
        </div>
      {/if}
    </div>
    <footer class="composer-wrap">
      {#if attachments.length > 0}<div class="attachment-list">{#each attachments as attachment (attachment.id)}<span class="attachment-chip"><span>▧</span>{attachment.name}<button aria-label={`Remove ${attachment.name}`} onclick={() => removeAttachment(attachment.id)}>×</button></span>{/each}</div>{/if}
      <div class="composer"><button class="attach-button" aria-label="Attach images" onclick={chooseImages}><Paperclip size={18} strokeWidth={1.8} /></button><textarea bind:this={composer} bind:value={draft} aria-label="Message" oninput={resizeComposer} onkeydown={handleComposerKeydown} placeholder="Message FM Chat" rows="1"></textarea>{#if isGenerating}<button class="stop-button" aria-label="Stop generating" onclick={cancelGeneration}><Square size={12} fill="currentColor" /></button>{:else}<button class="send-button" aria-label="Send message" disabled={!canSend} onclick={sendMessage}><ArrowUp size={17} strokeWidth={2.3} /></button>{/if}</div>
      <p class="composer-note">⌘↵ to send</p>
    </footer>
    </section>
  </main>
</div>

{#if conversationToDelete}
  <div class="confirmation-backdrop" role="presentation">
    <dialog class="confirmation-dialog" aria-labelledby="delete-title" aria-modal="true" open>
      <div class="confirmation-icon"><Trash2 size={19} strokeWidth={1.8} /></div>
      <h3 id="delete-title">Delete this chat?</h3>
      <p>“{conversationToDelete.title}” and its local images will be permanently removed from this Mac.</p>
      <div class="confirmation-actions"><button class="cancel-delete" onclick={dismissDeletion}>Cancel</button><button class="confirm-delete" onclick={confirmDeletion}>Delete chat</button></div>
    </dialog>
  </div>
{/if}
