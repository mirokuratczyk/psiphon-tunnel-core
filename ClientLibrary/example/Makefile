main: libpsiphontunnel.so main.o
	clang -I. -o main main.o -L. -lpsiphontunnel -framework IOKit -framework CoreFoundation

main.o: main.mm
	clang -I.. -c main.mm

libpsiphontunnel.so: ../PsiphonTunnel.go
	# At this time, we don't support modules
	GO111MODULE=off go build -buildmode=c-shared -o libpsiphontunnel.so ../PsiphonTunnel.go

clean:
	rm libpsiphontunnel.so libpsiphontunnel.h main main.o

