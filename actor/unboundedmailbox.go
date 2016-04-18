package actor

import "sync/atomic"
import "github.com/Workiva/go-datastructures/queue"

type QueueMailbox struct {
	userMailbox     *queue.Queue
	systemMailbox   *queue.Queue
	schedulerStatus int32
	hasMoreMessages int32
	actorCell       *ActorCell
}

func (mailbox *QueueMailbox) PostUserMessage(message interface{}) {
	mailbox.userMailbox.Put(message)
	mailbox.schedule()
}

func (mailbox *QueueMailbox) PostSystemMessage(message interface{}) {
	mailbox.systemMailbox.Put(message)
	mailbox.schedule()
}

func (mailbox *QueueMailbox) schedule() {
	swapped := atomic.CompareAndSwapInt32(&mailbox.schedulerStatus, MailboxIdle, MailboxRunning)
	atomic.StoreInt32(&mailbox.hasMoreMessages, MailboxHasMoreMessages) //we have more messages to process
	if swapped {
		go mailbox.processMessages()
	}
}

func (mailbox *QueueMailbox) processMessages() {
	//we are about to start processing messages, we can safely reset the message flag of the mailbox
	atomic.StoreInt32(&mailbox.hasMoreMessages, MailboxHasNoMessages)

	//process x messages in sequence, then exit
	for i := 0; i < 30; i++ {
		if !mailbox.systemMailbox.Empty() {
			sysMsg, _ := mailbox.systemMailbox.Get(1)
			first := sysMsg[0]
			mailbox.actorCell.invokeSystemMessage(first)
		} else if !mailbox.userMailbox.Empty() {
			userMsg, _ := mailbox.userMailbox.Get(1)
			first := userMsg[0]
			mailbox.actorCell.invokeUserMessage(first)
		}
	}
	//set mailbox to idle
	atomic.StoreInt32(&mailbox.schedulerStatus, MailboxIdle)
	//check if there are still messages to process (sent after the message loop ended)
	hasMore := atomic.LoadInt32(&mailbox.hasMoreMessages)
	//what is the current status of the mailbox? it could have changed concurrently since the last two lines
	status := atomic.LoadInt32(&mailbox.schedulerStatus)
	//if there are still messages to process and the mailbox is idle, then reschedule a mailbox run
	//otherwise, we either exit, or the mailbox have been scheduled already by the schedule method
	if hasMore == MailboxHasMoreMessages && status == MailboxIdle {
		swapped := atomic.CompareAndSwapInt32(&mailbox.schedulerStatus, MailboxIdle, MailboxRunning)
		if swapped {
			go mailbox.processMessages()
		}
	}
}

func NewQueueMailbox(cell *ActorCell) Mailbox {
	userMailbox := queue.New(10)
	systemMailbox := queue.New(10)
	mailbox := QueueMailbox{
		userMailbox:     userMailbox,
		systemMailbox:   systemMailbox,
		hasMoreMessages: MailboxHasNoMessages,
		schedulerStatus: MailboxIdle,
		actorCell:       cell,
	}
	return &mailbox
}