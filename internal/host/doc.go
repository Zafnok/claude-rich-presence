// Package host is the presence host: election, serving followers, failover,
// and the wiring from events to Discord updates. Everything outside reaches it
// through ports.
//
// It may import the core packages and the codecs. It must not import
// internal/cli, any adapter, or a transport package directly.
//
// An adapter holds one [Node]. It gives the node its events with Publish,
// which never blocks, and the node either is the presence host or follows
// one (ADR-0005). Which of the two is decided by an operating-system lock,
// never by this package: a node is host exactly while it holds the lock.
//
// # Roles
//
// Run moves the node between these states, one at a time:
//
//	                 lock taken                 valid stand_down
//	   Electing ------------------> Hosting ---------------------> Yielding
//	    ^   |                          |                               |
//	    |   | lock not taken           | a goroutine of the            |
//	    |   v                          | term panicked                 |
//	    |  Joining ---------------+    |                               |
//	    |   |      dial failed,   |    |                               |
//	    |   |      refused, or    v    v                               |
//	    |   | welcome  no welcome Retrying                             |
//	    |   v                     ^    |                               |
//	    |  Following -------------+    |                               |
//	    |        connection lost       |                               |
//	    +------------------------------+-------------------------------+
//	      the backoff, or the stand-down delay, has passed
//
//	every state --- context ended ---> Stopped
//
// The transitions, each of which has a test named after it:
//
//	Electing   the lock is taken                          Hosting
//	Electing   the lock is held, or cannot be tried       Joining
//	Joining    the host answers welcome                   Following
//	Joining    the dial fails                             Retrying
//	Joining    the host refuses                           Retrying
//	Joining    the connection ends without a welcome      Retrying
//	Following  the connection is lost                     Retrying
//	Hosting    a valid stand_down arrives                 Yielding
//	Hosting    a goroutine of the term panics             Retrying
//	Retrying   the jittered backoff has passed            Electing
//	Yielding   the stand-down delay has passed            Electing
//	any        the context ends                           Stopped
//
// Retrying waits between half of a delay and all of it. The delay starts at
// retryBase, doubles with each round that reaches no host, and starts again
// once a host has been reached. Yielding waits standDownDelay, which is
// several times retryBase, so that every follower has retried the lock before
// the node that gave it up does.
//
// A node that is newer than the host it has just lost, having asked it to
// stand down, tries the lock a few times in quick succession, sooner than
// any other follower's first wait can end, so that it is the one to take
// over.
//
// Standing down must not turn into giving the lock up over and over for a
// node that does not take it. So a node that cannot try the lock does not
// ask a host to stand down, and a node that has stood down does not do so
// again until it has followed a newer host: until then, each term it serves
// ignores stand_down.
//
// # The node's own session
//
// A node has one session at a time, and always holds its current state,
// reduced from the events it was given. That is what makes the host's state
// disposable. Whenever the node gains a host, itself included, the first
// thing it sends is that state as a sync, and events follow. The events wait
// in a bounded queue; when the queue is full they are dropped and a sync is
// sent in their place.
//
// The node's own session reaches the registry through the same messages as a
// follower's, so a host treats itself as one more source.
//
// # A term as host
//
// Each goroutine has one owner that waits for it:
//
//   - The goroutine in Run forwards the node's own session and ends the term.
//   - One goroutine owns the registry. Everything reaches it through one
//     channel. After every change it renders, hands the result to the
//     Discord connection, whose scheduler limits the rate, and arms a timer
//     for the moment the idle period, or a pause, would end.
//   - One goroutine runs the Discord connection.
//   - One goroutine listens and accepts. If the control socket cannot be
//     opened, or fails, it tries again with backoff, and meanwhile the node
//     hosts its own session alone.
//   - One goroutine per connection reads its lines, so a follower that
//     stalls delays nobody else. A connection owns one session: the one
//     its latest event or sync named. It is removed when the connection
//     closes. Only a sync moves a session from one connection to another.
//     There are at most maxConns connections; one more is closed as it is
//     accepted.
//   - One goroutine announces a change of pause to every welcomed
//     connection. A follower that has stopped reading holds it up, and
//     nothing else.
//
// A stand_down is valid when it comes from a newer binary: on a welcomed
// connection, one whose hello carried a newer binary version, and on a
// refused connection, one whose hello carried a newer protocol version. Any
// other is ignored, so two processes can never ask each other in turn.
//
// A term ends in this order: stop accepting, clear presence, close follower
// connections, close the listener, release the lock.
//
// # Pausing
//
// A pause switches the whole presence off without changing any setting
// (ADR-0012). The registry holds the latest pause or resume it was offered,
// and while a pause lasts it shows nothing, whatever the sessions are, and
// goes on reducing them. A pause must outlive the host, so every node keeps
// the latest pause or resume it knows of: its own request, or what its host
// said. Whenever it gains a host, itself included, it offers that first, and
// a term begins with it. See pause.go.
//
// # Never impairing the caller
//
// Publish, Status, Pause, Resume and Preview take a lock that is held only
// to read and write memory, and never wait for a goroutine that performs
// I/O. A follower's Status and
// Preview are what the host last said: the node asks again whenever it has
// sent something and whenever either is called, and answers from memory.
//
// A panic in a goroutine of this package is recovered and logged (ADR-0008).
// It costs the connection it happened on, or the term, and never the process.
package host
