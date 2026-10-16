package main

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: check_compat old.desc new.desc")
	}
	old := read(os.Args[1])
	now := read(os.Args[2])
	errs := 0
	for name, oldFile := range old {
		newFile := now[name]
		if newFile == nil {
			fmt.Fprintln(os.Stderr, "removed file:", name)
			errs++
			continue
		}
		oldMessages := messages(oldFile.GetMessageType(), oldFile.GetPackage())
		newMessages := messages(newFile.GetMessageType(), newFile.GetPackage())
		for path, oldMsg := range oldMessages {
			newMsg := newMessages[path]
			if newMsg == nil {
				fmt.Fprintln(os.Stderr, "removed message:", path)
				errs++
				continue
			}
			byNumber := map[int32]*descriptorpb.FieldDescriptorProto{}
			byName := map[string]*descriptorpb.FieldDescriptorProto{}
			for _, f := range newMsg.Field {
				byNumber[f.GetNumber()] = f
				byName[f.GetName()] = f
			}
			for _, f := range oldMsg.Field {
				next := byNumber[f.GetNumber()]
				if next == nil {
					if !reserved(newMsg, f) {
						fmt.Fprintln(os.Stderr, "removed field without reservation:", path, f.GetName())
						errs++
					}
					continue
				}
				if next.GetName() != f.GetName() || next.GetType() != f.GetType() || next.GetTypeName() != f.GetTypeName() || next.GetLabel() != f.GetLabel() || next.GetProto3Optional() != f.GetProto3Optional() {
					fmt.Fprintln(os.Stderr, "changed field:", path, f.GetName())
					errs++
				}
				if byName[f.GetName()] != next {
					fmt.Fprintln(os.Stderr, "reused field name:", path, f.GetName())
					errs++
				}
			}
		}
		oldEnums := enums(oldFile)
		newEnums := enums(newFile)
		for path, oldEnum := range oldEnums {
			next := newEnums[path]
			if next == nil {
				fmt.Fprintln(os.Stderr, "removed enum:", path)
				errs++
				continue
			}
			byNumber := map[int32]string{}
			for _, v := range next.Value {
				byNumber[v.GetNumber()] = v.GetName()
			}
			for _, v := range oldEnum.Value {
				if byNumber[v.GetNumber()] != v.GetName() {
					fmt.Fprintln(os.Stderr, "changed enum value:", path, v.GetName())
					errs++
				}
			}
		}
		byService := map[string]*descriptorpb.ServiceDescriptorProto{}
		for _, svc := range newFile.Service {
			byService[svc.GetName()] = svc
		}
		for _, svc := range oldFile.Service {
			next := byService[svc.GetName()]
			if next == nil {
				fmt.Fprintln(os.Stderr, "removed service:", svc.GetName())
				errs++
				continue
			}
			byMethod := map[string]*descriptorpb.MethodDescriptorProto{}
			for _, m := range next.Method {
				byMethod[m.GetName()] = m
			}
			for _, m := range svc.Method {
				n := byMethod[m.GetName()]
				if n == nil || n.GetInputType() != m.GetInputType() || n.GetOutputType() != m.GetOutputType() || n.GetClientStreaming() != m.GetClientStreaming() || n.GetServerStreaming() != m.GetServerStreaming() {
					fmt.Fprintln(os.Stderr, "changed RPC:", svc.GetName(), m.GetName())
					errs++
				}
			}
		}
	}
	if errs > 0 {
		os.Exit(1)
	}
}
func read(path string) map[string]*descriptorpb.FileDescriptorProto {
	raw, e := os.ReadFile(path)
	if e != nil {
		panic(e)
	}
	set := new(descriptorpb.FileDescriptorSet)
	if e = proto.Unmarshal(raw, set); e != nil {
		panic(e)
	}
	out := map[string]*descriptorpb.FileDescriptorProto{}
	for _, f := range set.File {
		out[f.GetName()] = f
	}
	return out
}
func messages(list []*descriptorpb.DescriptorProto, prefix string) map[string]*descriptorpb.DescriptorProto {
	out := map[string]*descriptorpb.DescriptorProto{}
	for _, m := range list {
		path := prefix + "." + m.GetName()
		out[path] = m
		for k, v := range messages(m.NestedType, path) {
			out[k] = v
		}
	}
	return out
}
func enums(f *descriptorpb.FileDescriptorProto) map[string]*descriptorpb.EnumDescriptorProto {
	out := map[string]*descriptorpb.EnumDescriptorProto{}
	for _, v := range f.EnumType {
		out[f.GetPackage()+"."+v.GetName()] = v
	}
	return out
}
func reserved(m *descriptorpb.DescriptorProto, f *descriptorpb.FieldDescriptorProto) bool {
	number := false
	name := false
	for _, r := range m.ReservedRange {
		if f.GetNumber() >= r.GetStart() && f.GetNumber() < r.GetEnd() {
			number = true
		}
	}
	for _, n := range m.ReservedName {
		if n == f.GetName() {
			name = true
		}
	}
	return number && name
}
