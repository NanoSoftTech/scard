package scard

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	modwinscard = syscall.NewLazyDLL("winscard.dll")

	procEstablishContext = modwinscard.NewProc("SCardEstablishContext")
	procRelease          = modwinscard.NewProc("SCardReleaseContext")
	procIsValid          = modwinscard.NewProc("SCardIsValidContext")
	procCancel           = modwinscard.NewProc("SCardCancel")
	procListReaders      = modwinscard.NewProc("SCardListReadersW")
	procListReaderGroups = modwinscard.NewProc("SCardListReaderGroupsW")
	procGetStatusChange  = modwinscard.NewProc("SCardGetStatusChangeW")
	procConnect          = modwinscard.NewProc("SCardConnectW")
	procDisconnect       = modwinscard.NewProc("SCardDisconnect")
	procReconnect        = modwinscard.NewProc("SCardReconnect")
	procBeginTransaction = modwinscard.NewProc("SCardBeginTransaction")
	procEndTransaction   = modwinscard.NewProc("SCardEndTransaction")
	procStatus           = modwinscard.NewProc("SCardStatusW")
	procTransmit         = modwinscard.NewProc("SCardTransmit")
	procControl          = modwinscard.NewProc("SCardControl")
	procGetAttrib        = modwinscard.NewProc("SCardGetAttrib")
	procSetAttrib        = modwinscard.NewProc("SCardSetAttrib")

	dataT0Pci = modwinscard.NewProc("g_rgSCardT0Pci")
	dataT1Pci = modwinscard.NewProc("g_rgSCardT1Pci")
)

var scardIoReqT0 uintptr
var scardIoReqT1 uintptr

func init() {
	if err := dataT0Pci.Find(); err != nil {
		panic(err)
	}
	scardIoReqT0 = dataT0Pci.Addr()
	if err := dataT1Pci.Find(); err != nil {
		panic(err)
	}
	scardIoReqT1 = dataT1Pci.Addr()
}

type Context struct {
	ctx uintptr
}

type Card struct {
	handle         uintptr
	activeProtocol uintptr
}

func (e Error) Error() string {
	err := syscall.Errno(e)
	return fmt.Sprintf("scard: error(%x): %s", uintptr(e), err.Error())
}

// wraps SCardEstablishContext
func EstablishContext() (*Context, error) {
	var ctx Context

	r, _, _ := procEstablishContext.Call(2, uintptr(0), uintptr(0), uintptr(unsafe.Pointer(&ctx.ctx)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	return &ctx, nil
}

// wraps SCardIsValidContext
func (ctx *Context) IsValid() (bool, error) {
	r, _, _ := procIsValid.Call(ctx.ctx)
	if Error(r) != ErrSuccess {
		return false, Error(r)
	}
	return true, nil
}

// wraps SCardCancel
func (ctx *Context) Cancel() error {
	r, _, _ := procCancel.Call(ctx.ctx)
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}

// wraps SCardReleaseContext
func (ctx *Context) Release() error {
	r, _, _ := procRelease.Call(uintptr(ctx.ctx))
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}

// wraps SCardListReaders
func (ctx *Context) ListReaders() ([]string, error) {
	var needed uintptr

	r, _, _ := procListReaders.Call(
		ctx.ctx,
		0,
		0,
		uintptr(unsafe.Pointer(&needed)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	data := make([]uint16, needed)
	r, _, _ = procListReaders.Call(
		ctx.ctx,
		0,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&needed)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	var readers []string
	for len(data) > 0 {
		pos := 0
		for ; pos < len(data); pos++ {
			if data[pos] == 0 {
				break
			}
		}
		if data[0] != 0 {
			reader := syscall.UTF16ToString(data[:pos])
			readers = append(readers, reader)
		}
		data = data[pos+1:]
	}

	return readers, nil
}

// wraps SCardListReaderGroups
func (ctx *Context) ListReaderGroups() ([]string, error) {
	var needed uintptr

	r, _, _ := procListReaderGroups.Call(
		ctx.ctx,
		0,
		uintptr(unsafe.Pointer(&needed)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	data := make([]uint16, needed)
	r, _, _ = procListReaderGroups.Call(
		ctx.ctx,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&needed)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	var groups []string
	for len(data) > 0 {
		pos := 0
		for ; pos < len(data); pos++ {
			if data[pos] == 0 {
				break
			}
		}
		if data[0] != 0 {
			group := syscall.UTF16ToString(data[:pos])
			groups = append(groups, group)
		}
		data = data[pos+1:]
	}

	return groups, nil
}

type scardReaderState struct {
	szReader       uintptr
	pvUserData     uintptr
	dwCurrentState uint32
	dwEventState   uint32
	cbAtr          uint32
	rgbAtr         [36]byte
}

// wraps SCardGetStatusChange
func (ctx *Context) GetStatusChange(readerStates []ReaderState, timeout time.Duration) error {
	var dwTimeout uint32

	switch {
	case timeout < 0:
		dwTimeout = infiniteTimeout
	case timeout > time.Duration(infiniteTimeout)*time.Millisecond:
		dwTimeout = infiniteTimeout - 1
	default:
		dwTimeout = uint32(timeout / time.Millisecond)
	}

	crs := make([]scardReaderState, len(readerStates))

	for i := range readerStates {
		rptr, err := syscall.UTF16PtrFromString(readerStates[i].Reader)
		if err != nil {
			return err
		}
		crs[i].szReader = uintptr(unsafe.Pointer(rptr))
		crs[i].dwCurrentState = uint32(readerStates[i].CurrentState)
		crs[i].cbAtr = uint32(len(readerStates[i].Atr))
		copy(crs[i].rgbAtr[:], readerStates[i].Atr)
	}

	r, _, _ := procGetStatusChange.Call(
		ctx.ctx,
		uintptr(dwTimeout),
		uintptr(unsafe.Pointer(&crs[0])),
		uintptr(len(crs)))

	if Error(r) != ErrSuccess {
		return Error(r)
	}

	for i := range readerStates {
		readerStates[i].EventState = StateFlag(crs[i].dwEventState)
		if crs[i].cbAtr > 0 {
			readerStates[i].Atr = make([]byte, int(crs[i].cbAtr))
			copy(readerStates[i].Atr, crs[i].rgbAtr[:crs[i].cbAtr])
		}
	}

	return nil
}

// wraps SCardConnect
func (ctx *Context) Connect(reader string, mode ShareMode, proto Protocol) (*Card, error) {
	var card Card

	creader, err := syscall.UTF16PtrFromString(reader)
	if err != nil {
		return nil, err
	}

	r, _, _ := procConnect.Call(
		ctx.ctx,
		uintptr(unsafe.Pointer(creader)),
		uintptr(mode),
		uintptr(proto),
		uintptr(unsafe.Pointer(&card.handle)),
		uintptr(unsafe.Pointer(&card.activeProtocol)))

	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	return &card, nil
}

// wraps SCardDisconnect
func (card *Card) Disconnect(d Disposition) error {
	r, _, _ := procDisconnect.Call(card.handle, uintptr(d))
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}

// wraps SCardReconnect
func (card *Card) Reconnect(mode ShareMode, protocol Protocol, init Disposition) error {
	r, _, _ := procReconnect.Call(card.handle, uintptr(protocol), uintptr(init))
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}

// wraps SCardBeginTransaction
func (card *Card) BeginTransaction() error {
	r, _, _ := procBeginTransaction.Call(card.handle)
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}

// wraps SCardEndTransaction
func (card *Card) EndTransaction(d Disposition) error {
	r, _, _ := procEndTransaction.Call(card.handle, uintptr(d))
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}

// wraps SCardStatus
func (card *Card) Status() (*CardStatus, error) {
	var reader [maxReadername + 1]uint16
	var readerLen = uint32(len(reader))
	var state, proto uint32
	var atr [maxAtrSize]byte
	var atrLen = uint32(len(atr))

	r, _, _ := procStatus.Call(
		card.handle,
		uintptr(unsafe.Pointer(&reader[0])),
		uintptr(unsafe.Pointer(&readerLen)),
		uintptr(unsafe.Pointer(&state)),
		uintptr(unsafe.Pointer(&proto)),
		uintptr(unsafe.Pointer(&atr[0])),
		uintptr(unsafe.Pointer(&atrLen)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	status := &CardStatus{
		Reader:         syscall.UTF16ToString(reader[0:readerLen]),
		State:          State(state),
		ActiveProtocol: Protocol(proto),
		Atr:            atr[0:atrLen],
	}

	return status, nil
}

// wraps SCardTransmit
func (card *Card) Transmit(cmd []byte) ([]byte, error) {
	var sendpci uintptr

	switch Protocol(card.activeProtocol) {
	case ProtocolT0:
		sendpci = scardIoReqT0
	case ProtocolT1:
		sendpci = scardIoReqT1
	default:
		panic("unknown protocol")
	}

	var recv [maxBufferSizeExtended]byte
	var recvlen = uint32(len(recv))

	r, _, _ := procTransmit.Call(card.handle,
		sendpci,
		uintptr(unsafe.Pointer(&cmd[0])),
		uintptr(len(cmd)),
		0,
		uintptr(unsafe.Pointer(&recv[0])),
		uintptr(unsafe.Pointer(&recvlen)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	rsp := make([]byte, recvlen)
	copy(rsp, recv[0:recvlen])

	return rsp, nil
}

// wraps SCardControl
func (card *Card) Control(ctrl uint32, cmd []byte) ([]byte, error) {
	var recv [0xffff]byte
	var recvlen uintptr
	var r uintptr

	if len(cmd) == 0 {
		r, _, _ = procControl.Call(
			card.handle,
			uintptr(ctrl),
			0,
			0,
			uintptr(unsafe.Pointer(&recv[0])),
			uintptr(len(recv)),
			uintptr(unsafe.Pointer(&recvlen)))
	} else {
		r, _, _ = procControl.Call(
			card.handle,
			uintptr(ctrl),
			uintptr(unsafe.Pointer(&cmd[0])),
			uintptr(len(cmd)),
			uintptr(unsafe.Pointer(&recv[0])),
			uintptr(len(recv)),
			uintptr(unsafe.Pointer(&recvlen)))
	}
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	rsp := make([]byte, recvlen)
	copy(rsp, recv[0:recvlen])

	return rsp, nil
}

// wraps SCardGetAttrib
func (card *Card) GetAttrib(id Attrib) ([]byte, error) {
	var needed uintptr

	r, _, _ := procGetAttrib.Call(
		card.handle,
		uintptr(id),
		0,
		uintptr(unsafe.Pointer(&needed)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	var attrib = make([]byte, needed)

	r, _, _ = procGetAttrib.Call(
		card.handle,
		uintptr(id),
		uintptr(unsafe.Pointer(&attrib[0])),
		uintptr(unsafe.Pointer(&needed)))
	if Error(r) != ErrSuccess {
		return nil, Error(r)
	}

	return attrib[0:needed], nil
}

// wraps SCardSetAttrib
func (card *Card) SetAttrib(id Attrib, data []byte) error {
	r, _, _ := procSetAttrib.Call(
		card.handle,
		uintptr(id),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)))
	if Error(r) != ErrSuccess {
		return Error(r)
	}
	return nil
}
